package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.model.DeadDropPayload;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PostConstruct;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.data.redis.connection.stream.Consumer;
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.ReadOffset;
import org.springframework.data.redis.connection.stream.StreamOffset;
import org.springframework.data.redis.connection.stream.StreamReadOptions;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Persists worker terminal audit envelopes from a replayable Redis Stream.
 * Records are acknowledged and deleted only after PostgreSQL persistence.
 */
@Service
@ConditionalOnProperty(name = "coldharbor.redis.durable-consumers-enabled",
        havingValue = "true", matchIfMissing = true)
public class DurableAuditStreamConsumer {

    private static final Logger log = LoggerFactory.getLogger(DurableAuditStreamConsumer.class);
    private static final DefaultRedisScript<Long> CREATE_GROUP_SCRIPT = new DefaultRedisScript<>(
            """
            local result = redis.pcall('XGROUP', 'CREATE', KEYS[1], ARGV[1], '0', 'MKSTREAM')
            if type(result) == 'table' and result.err and
               not string.find(result.err, 'BUSYGROUP', 1, true) then
              return redis.error_reply(result.err)
            end
            return 1
            """, Long.class);

    private final StringRedisTemplate redisTemplate;
    private final AuditService auditService;
    private final ObjectMapper objectMapper;

    @Value("${coldharbor.redis.audit-stream-name:coldharbor:audits}")
    private String streamName;

    @Value("${coldharbor.redis.audit-consumer-group:control-plane-audit}")
    private String consumerGroup;

    @Value("${coldharbor.redis.audit-consumer-name:audit-writer}")
    private String consumerName;

    private volatile boolean groupReady;

    public DurableAuditStreamConsumer(StringRedisTemplate redisTemplate,
                                      AuditService auditService,
                                      ObjectMapper objectMapper) {
        this.redisTemplate = redisTemplate;
        this.auditService = auditService;
        this.objectMapper = objectMapper;
    }

    @PostConstruct
    public void initializeGroup() {
        try {
            redisTemplate.execute(CREATE_GROUP_SCRIPT, List.of(streamName), consumerGroup);
            groupReady = true;
        } catch (Exception e) {
            groupReady = false;
            log.warn("Audit stream consumer group initialization deferred: {}", e.getMessage());
        }
    }

    @Scheduled(fixedDelayString = "${coldharbor.redis.audit-poll-ms:500}")
    public void poll() {
        if (!groupReady) {
            initializeGroup();
            if (!groupReady) {
                return;
            }
        }

        // Drain this logical consumer's unacknowledged records first, which
        // makes restarts replay-safe, then claim new group records.
        int processed = processBatch(ReadOffset.from("0"));
        if (processed == 0) {
            processBatch(ReadOffset.lastConsumed());
        }
    }

    private int processBatch(ReadOffset offset) {
        List<MapRecord<String, Object, Object>> records;
        try {
            records = redisTemplate.opsForStream().read(
                    Consumer.from(consumerGroup, consumerName),
                    StreamReadOptions.empty().count(50),
                    StreamOffset.create(streamName, offset));
        } catch (Exception e) {
            groupReady = false;
            log.warn("Audit stream read failed: {}", e.getMessage());
            return 0;
        }
        if (records == null) {
            return 0;
        }
        for (MapRecord<String, Object, Object> record : records) {
            persistAndAcknowledge(record);
        }
        return records.size();
    }

    private void persistAndAcknowledge(MapRecord<String, Object, Object> record) {
        try {
            Map<Object, Object> fields = new LinkedHashMap<>(record.getValue());
            DeadDropPayload payload = objectMapper.readValue(
                    String.valueOf(fields.get("data")), DeadDropPayload.class);
            String finalState = fields.containsKey("finalState")
                    ? String.valueOf(fields.get("finalState")) : "PURGED";
            String details = fields.containsKey("details")
                    ? String.valueOf(fields.get("details")) : "";
            auditService.recordCompletedJob(payload.getCompartmentId(), payload, details, finalState);
            redisTemplate.opsForStream().acknowledge(streamName, consumerGroup, record.getId());
            redisTemplate.opsForStream().delete(streamName, record.getId());
        } catch (Exception e) {
            log.warn("Audit stream record {} remains pending after persistence failure: {}",
                    record.getId(), e.getMessage());
        }
    }
}
