package com.coldharbor.controlplane.service;

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

import java.util.List;

/**
 * Replays lifecycle events into the compartment query projection when live
 * Pub/Sub delivery was missed.
 */
@Service
@ConditionalOnProperty(name = "coldharbor.redis.durable-consumers-enabled",
        havingValue = "true", matchIfMissing = true)
public class DurableEventStreamConsumer {

    private static final Logger log = LoggerFactory.getLogger(DurableEventStreamConsumer.class);
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
    private final EventRelayService eventRelayService;

    @Value("${coldharbor.redis.event-stream-name:coldharbor:events:stream}")
    private String streamName;

    @Value("${coldharbor.redis.event-consumer-group:control-plane-events}")
    private String consumerGroup;

    @Value("${coldharbor.redis.event-consumer-name:event-projector}")
    private String consumerName;

    private volatile boolean groupReady;

    public DurableEventStreamConsumer(StringRedisTemplate redisTemplate,
                                      EventRelayService eventRelayService) {
        this.redisTemplate = redisTemplate;
        this.eventRelayService = eventRelayService;
    }

    @PostConstruct
    public void initializeGroup() {
        try {
            redisTemplate.execute(CREATE_GROUP_SCRIPT, List.of(streamName), consumerGroup);
            groupReady = true;
        } catch (Exception e) {
            groupReady = false;
            log.warn("Event stream consumer group initialization deferred: {}", e.getMessage());
        }
    }

    @Scheduled(fixedDelayString = "${coldharbor.redis.event-poll-ms:500}")
    public void poll() {
        if (!groupReady) {
            initializeGroup();
            if (!groupReady) {
                return;
            }
        }
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
                    StreamReadOptions.empty().count(100),
                    StreamOffset.create(streamName, offset));
        } catch (Exception e) {
            groupReady = false;
            log.warn("Event stream read failed: {}", e.getMessage());
            return 0;
        }
        if (records == null) {
            return 0;
        }
        for (MapRecord<String, Object, Object> record : records) {
            try {
                eventRelayService.processDurableEvent(String.valueOf(record.getValue().get("data")));
                redisTemplate.opsForStream().acknowledge(streamName, consumerGroup, record.getId());
                redisTemplate.opsForStream().delete(streamName, record.getId());
            } catch (Exception e) {
                log.warn("Event stream record {} remains pending after projection failure: {}",
                        record.getId(), e.getMessage());
            }
        }
        return records.size();
    }
}
