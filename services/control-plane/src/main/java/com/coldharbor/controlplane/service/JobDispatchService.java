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
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.RecordId;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

/**
 * Service responsible for creating compartments and dispatching jobs to Redis Streams.
 */
@Service
public class JobDispatchService {

    private static final Logger log = LoggerFactory.getLogger(JobDispatchService.class);

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

        // 1. Dispatch to Redis Stream coldharbor:jobs
        try {
            Map<String, String> streamFields = new HashMap<>();
            streamFields.put("compartmentId", jobMessage.getCompartmentId());
            streamFields.put("context", jobMessage.getContext());
            streamFields.put("ownerId", jobMessage.getOwnerId());
            streamFields.put("taskType", jobMessage.getTaskType());
            streamFields.put("maxRetries", String.valueOf(jobMessage.getMaxRetries()));
            streamFields.put("timeoutSeconds", String.valueOf(jobMessage.getTimeoutSeconds()));
            streamFields.put("createdAt", jobMessage.getCreatedAt().toString());

            if (jobMessage.getPayload() != null) {
                streamFields.put("payload", objectMapper.writeValueAsString(jobMessage.getPayload()));
            } else {
                streamFields.put("payload", "{}");
            }

            // Also provide unified JSON representation
            streamFields.put("data", objectMapper.writeValueAsString(jobMessage));

            RecordId recordId = redisTemplate.opsForStream().add(
                    MapRecord.create(streamName, streamFields)
            );
            log.info("Dispatched job {} to stream {} with recordId {}", compartmentId, streamName, recordId);
        } catch (Exception e) {
            log.error("Failed to publish job to Redis stream {}: {}", streamName, e.getMessage(), e);
            throw new RuntimeException("Failed to dispatch job to Redis stream: " + e.getMessage(), e);
        }

        // 2. Persist initial compartment metadata in Redis
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

        saveCompartmentMeta(detail);
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
