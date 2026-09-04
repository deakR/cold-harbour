package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.WorkerStatusResponse;
import com.coldharbor.controlplane.model.HeartbeatPayload;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Worker Wellness Monitor periodically scanning worker:*:heartbeat keys in Redis.
 */
@Service
public class WorkerWellnessMonitor {

    private static final Logger log = LoggerFactory.getLogger(WorkerWellnessMonitor.class);
    private static final long HEARTBEAT_TIMEOUT_SECONDS = 30L;

    private final StringRedisTemplate redisTemplate;
    private final ObjectMapper objectMapper;

    @Value("${coldharbor.redis.heartbeat-pattern:worker:*:heartbeat}")
    private String heartbeatPattern;

    @Value("${coldharbor.redis.dlq-stream-name:coldharbor:jobs:dlq}")
    private String dlqStreamName;

    @Value("${coldharbor.redis.stream-name:coldharbor:jobs}")
    private String streamName;

    private final Map<String, WorkerStatusResponse> workerRegistry = new ConcurrentHashMap<>();

    public WorkerWellnessMonitor(StringRedisTemplate redisTemplate, ObjectMapper objectMapper) {
        this.redisTemplate = redisTemplate;
        this.objectMapper = objectMapper;
    }

    /**
     * Periodic scan of worker heartbeat keys.
     */
    @Scheduled(fixedRate = 10000)
    public void scanWorkerHeartbeats() {
        try {
            refreshWorkerStatus();
        } catch (Exception e) {
            log.warn("Error scanning worker heartbeats: {}", e.getMessage());
        }
    }

    public List<WorkerStatusResponse> getActiveWorkers() {
        try {
            refreshWorkerStatus();
        } catch (Exception e) {
            log.warn("Error during active workers refresh: {}", e.getMessage());
        }
        return new ArrayList<>(workerRegistry.values());
    }

    public void refreshWorkerStatus() {
        Instant now = Instant.now();
        Set<String> keys = Collections.emptySet();

        try {
            keys = redisTemplate.keys(heartbeatPattern);
        } catch (Exception e) {
            log.debug("Redis keys scan skipped or failed: {}", e.getMessage());
        }

        Set<String> activeKeys = (keys != null) ? keys : Collections.emptySet();

        for (String key : activeKeys) {
            try {
                String val = redisTemplate.opsForValue().get(key);
                if (val == null || val.trim().isEmpty()) {
                    continue;
                }

                HeartbeatPayload heartbeat = objectMapper.readValue(val, HeartbeatPayload.class);
                String workerId = heartbeat.getWorkerId();
                if (workerId == null || workerId.trim().isEmpty()) {
                    // Extract from key worker:{id}:heartbeat
                    String[] parts = key.split(":");
                    if (parts.length >= 2) {
                        workerId = parts[1];
                    } else {
                        workerId = key;
                    }
                }

                Instant hbTime = heartbeat.getTimestamp() != null ? heartbeat.getTimestamp() : now;
                long elapsedSeconds = Math.max(0, Duration.between(hbTime, now).toSeconds());

                boolean healthy = elapsedSeconds <= HEARTBEAT_TIMEOUT_SECONDS;
                String status = healthy ? (heartbeat.getStatus() != null ? heartbeat.getStatus() : "IDLE") : "DEAD";

                WorkerStatusResponse statusResp = new WorkerStatusResponse(
                        workerId,
                        status,
                        heartbeat.getActiveCompartmentId(),
                        hbTime,
                        healthy,
                        elapsedSeconds
                );

                if (!healthy) {
                    log.warn("Worker {} heartbeat timed out ({}s elapsed) - flagged DEAD", workerId, elapsedSeconds);
                }

                workerRegistry.put(workerId, statusResp);
            } catch (Exception e) {
                log.warn("Failed to parse heartbeat key {}: {}", key, e.getMessage());
            }
        }

        // Check existing registry for workers whose keys expired or disappeared
        for (Map.Entry<String, WorkerStatusResponse> entry : workerRegistry.entrySet()) {
            WorkerStatusResponse existing = entry.getValue();
            long elapsed = Math.max(0, Duration.between(existing.getLastHeartbeat(), now).toSeconds());
            if (elapsed > HEARTBEAT_TIMEOUT_SECONDS && existing.isHealthy()) {
                existing.setHealthy(false);
                existing.setStatus("DEAD");
                existing.setSecondsSinceLastHeartbeat(elapsed);
                log.warn("Worker {} key expired or timed out ({}s elapsed) - flagged DEAD", entry.getKey(), elapsed);
            }
        }
    }

    public void registerWorkerHeartbeat(String workerId, String status, String activeCompartmentId) {
        WorkerStatusResponse resp = new WorkerStatusResponse(
                workerId,
                status,
                activeCompartmentId,
                Instant.now(),
                true,
                0L
        );
        workerRegistry.put(workerId, resp);
    }

    /**
     * Reads dead-letter queue depth and most recent entries for operator inspection.
     */
    public Map<String, Object> getDlqSnapshot(int limit) {
        int capped = Math.min(Math.max(limit, 1), 100);
        long size = 0;
        List<Map<String, String>> entries = new ArrayList<>();
        try {
            Long len = redisTemplate.opsForStream().size(dlqStreamName);
            if (len != null) {
                size = len;
            }
            var records = redisTemplate.opsForStream().range(
                    dlqStreamName,
                    org.springframework.data.domain.Range.closed("0-0", "+"),
                    org.springframework.data.redis.connection.Limit.limit().count(capped));
            if (records != null) {
                for (var record : records) {
                    Map<String, String> entry = new java.util.LinkedHashMap<>();
                    entry.put("_streamId", record.getId().getValue());
                    record.getValue().forEach((k, v) ->
                            entry.put(String.valueOf(k), String.valueOf(v)));
                    entries.add(entry);
                }
            }
        } catch (Exception e) {
            log.debug("DLQ snapshot unavailable for {}: {}", dlqStreamName, e.getMessage());
        }
        Map<String, Object> snapshot = new java.util.LinkedHashMap<>();
        snapshot.put("stream", dlqStreamName);
        snapshot.put("size", size);
        snapshot.put("entries", entries);
        return snapshot;
    }

    /**
     * Requeues up to {@code limit} dead-letter entries onto the main stream for
     * another attempt (e.g. after fixing the transient cause), removing them from
     * the DLQ. Returns the redriven stream IDs. Poison pills that still fail will
     * land back in the DLQ through the normal retry path.
     */
    @SuppressWarnings("unchecked")
    public Map<String, Object> redriveDlq(int limit) {
        int capped = Math.min(Math.max(limit, 1), 100);
        List<String> redriven = new ArrayList<>();
        try {
            var ops = redisTemplate.opsForStream();
            var records = ops.range(
                    dlqStreamName,
                    org.springframework.data.domain.Range.closed("0-0", "+"),
                    org.springframework.data.redis.connection.Limit.limit().count(capped));
            if (records == null || records.isEmpty()) {
                return Map.of("redriven", 0, "ids", List.of());
            }
            List<org.springframework.data.redis.connection.stream.RecordId> ids = new ArrayList<>();
            for (var record : records) {
                Map<String, String> fields = new java.util.LinkedHashMap<>();
                record.getValue().forEach((k, v) -> fields.put(String.valueOf(k), String.valueOf(v)));
                String jobData = fields.get("jobData");
                if (jobData == null || jobData.trim().isEmpty()) {
                    continue;
                }
                Map<String, Object> job = objectMapper.readValue(jobData, Map.class);
                Map<String, String> streamFields = new java.util.LinkedHashMap<>();
                for (String key : new String[]{"compartmentId", "context", "ownerId", "taskType",
                        "maxRetries", "timeoutSeconds", "createdAt"}) {
                    Object val = job.get(key);
                    if (val != null) {
                        streamFields.put(key, String.valueOf(val));
                    }
                }
                Object payload = job.get("payload");
                streamFields.put("payload", payload != null ? objectMapper.writeValueAsString(payload) : "{}");
                streamFields.put("data", jobData);
                ops.add(org.springframework.data.redis.connection.stream.MapRecord.create(streamName, streamFields));
                ids.add(record.getId());
                redriven.add(String.valueOf(fields.getOrDefault("compartmentId", record.getId().getValue())));
            }
            if (!ids.isEmpty()) {
                ops.delete(dlqStreamName, ids.toArray(new org.springframework.data.redis.connection.stream.RecordId[0]));
            }
        } catch (Exception e) {
            log.warn("DLQ redrive failed: {}", e.getMessage());
            throw new RuntimeException("DLQ redrive failed: " + e.getMessage(), e);
        }
        Map<String, Object> result = new java.util.LinkedHashMap<>();
        result.put("redriven", redriven.size());
        result.put("ids", redriven);
        return result;
    }
}
