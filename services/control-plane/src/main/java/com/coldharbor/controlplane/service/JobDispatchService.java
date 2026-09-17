package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.CreateCompartmentRequest;
import com.coldharbor.controlplane.model.JobMessage;
import com.coldharbor.controlplane.security.ClearanceContext;
import com.coldharbor.controlplane.security.ContextClearance;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;

/**
 * Service responsible for creating compartments and dispatching jobs to Redis Streams.
 */
@Service
public class JobDispatchService {

    private static final Logger log = LoggerFactory.getLogger(JobDispatchService.class);
    private static final int MAX_PAYLOAD_BYTES = 1_048_576;
    private static final DefaultRedisScript<Long> DISPATCH_SCRIPT = new DefaultRedisScript<>(
            """
            if redis.call('EXISTS', KEYS[3]) == 1 then return 0 end
            redis.call('SET', KEYS[3], '1')
            local meta = redis.pcall('SET', KEYS[1], ARGV[1], 'EX', ARGV[2], 'NX')
            if type(meta) == 'table' and meta.err then
              redis.call('DEL', KEYS[3])
              return redis.error_reply(meta.err)
            end
            if not meta then
              redis.call('DEL', KEYS[3])
              return 0
            end
            local added = redis.pcall('XADD', KEYS[2], '*', unpack(ARGV, 3))
            if type(added) == 'table' and added.err then
              redis.call('DEL', KEYS[1], KEYS[3])
              return redis.error_reply(added.err)
            end
            return 1
            """, Long.class);

    private final StringRedisTemplate redisTemplate;
    private final ObjectMapper objectMapper;

    @Value("${coldharbor.redis.stream-name:coldharbor:jobs}")
    private String streamName;

    @Value("${coldharbor.redis.meta-prefix:compartment:}")
    private String metaPrefix;

    public JobDispatchService(StringRedisTemplate redisTemplate, ObjectMapper objectMapper) {
        this.redisTemplate = redisTemplate;
        this.objectMapper = objectMapper;
    }

    /**
     * Creates compartment and dispatches structured JobMessage to Redis Stream.
     */
    public CompartmentDetail dispatchJob(CreateCompartmentRequest request) {
        String compartmentId = request.getCompartmentId();
        if (compartmentId == null || compartmentId.trim().isEmpty()) {
            compartmentId = "cpt_" + UUID.randomUUID().toString().replace("-", "").substring(0, 12);
        }

        Instant now = Instant.now();
        JobMessage jobMessage = new JobMessage(
                compartmentId,
                request.getContext(),
                request.getOwnerId(),
                request.getTaskType(),
                request.getPayload(),
                request.getMaxRetries(),
                request.getTimeoutSeconds(),
                now
        );

        CompartmentDetail detail = new CompartmentDetail(
                compartmentId,
                jobMessage.getContext(),
                jobMessage.getOwnerId(),
                jobMessage.getTaskType(),
                "QUEUED",
                0,
                now,
                now,
                "Job dispatched to stream queue"
        );

        // Atomically reserve the ID, write QUEUED metadata, and publish the stream entry.
        try {
            Map<String, String> streamFields = new HashMap<>();
            streamFields.put("compartmentId", jobMessage.getCompartmentId());
            streamFields.put("context", jobMessage.getContext());
            streamFields.put("ownerId", jobMessage.getOwnerId());
            streamFields.put("taskType", jobMessage.getTaskType());
            streamFields.put("maxRetries", String.valueOf(jobMessage.getMaxRetries()));
            streamFields.put("timeoutSeconds", String.valueOf(jobMessage.getTimeoutSeconds()));
            streamFields.put("createdAt", jobMessage.getCreatedAt().toString());

            String payloadJson = jobMessage.getPayload() != null
                    ? objectMapper.writeValueAsString(jobMessage.getPayload()) : "{}";
            if (payloadJson.getBytes(java.nio.charset.StandardCharsets.UTF_8).length > MAX_PAYLOAD_BYTES) {
                throw new IllegalArgumentException("payload must not exceed 1 MiB when serialized");
            }
            streamFields.put("payload", payloadJson);

            // Also provide unified JSON representation
            streamFields.put("data", objectMapper.writeValueAsString(jobMessage));

            List<String> args = new ArrayList<>();
            args.add(objectMapper.writeValueAsString(detail));
            args.add(String.valueOf(Duration.ofHours(24).toSeconds()));
            streamFields.forEach((key, value) -> {
                args.add(key);
                args.add(value);
            });
            String metaKey = metaPrefix + compartmentId + ":meta";
            Long result = redisTemplate.execute(DISPATCH_SCRIPT,
                    List.of(metaKey, streamName, metaKey + ":dispatch"),
                    (Object[]) args.toArray(new String[0]));
            if (result == null || result == 0) {
                throw new IllegalArgumentException("compartmentId already exists: " + compartmentId);
            }
            log.info("Atomically dispatched job {} to stream {}", compartmentId, streamName);
        } catch (IllegalArgumentException e) {
            throw e;
        } catch (Exception e) {
            log.error("Failed to publish job to Redis stream {}: {}", streamName, e.getMessage(), e);
            throw new RuntimeException("Failed to dispatch job to Redis stream: " + e.getMessage(), e);
        }

        return detail;
    }

    public void saveCompartmentMeta(CompartmentDetail detail) {
        try {
            String key = metaPrefix + detail.getCompartmentId() + ":meta";
            String json = objectMapper.writeValueAsString(detail);
            redisTemplate.opsForValue().set(key, json, Duration.ofHours(24));
        } catch (Exception e) {
            log.warn("Failed to write compartment metadata to Redis: {}", e.getMessage());
        }
    }
}
