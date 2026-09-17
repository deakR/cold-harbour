package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.WorkerStatusResponse;
import com.coldharbor.controlplane.model.HeartbeatPayload;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.RedisCallback;
import org.springframework.data.redis.core.ScanOptions;
import org.springframework.data.redis.core.Cursor;
import org.springframework.data.redis.core.script.DefaultRedisScript;
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
    private static final long STALE_REGISTRY_RETENTION_SECONDS = 300L;
    @SuppressWarnings("rawtypes")
    private static final DefaultRedisScript<List> REDRIVE_SCRIPT = new DefaultRedisScript<>(
            """
            local records = redis.call('XRANGE', KEYS[1], '-', '+', 'COUNT', tonumber(ARGV[1]))
            local moved = {}
            for _, record in ipairs(records) do
              local fields = record[2]
              local jobData = nil
              local fallbackId = record[1]
              for i = 1, #fields, 2 do
                if fields[i] == 'jobData' then jobData = fields[i + 1] end
                if fields[i] == 'compartmentId' then fallbackId = fields[i + 1] end
              end
              if jobData then
                local valid, decoded = pcall(cjson.decode, jobData)
                if valid then
                  local compartmentId = decoded.compartmentId or fallbackId
                  local payload = decoded.payload and cjson.encode(decoded.payload) or '{}'
                  redis.call('XADD', KEYS[2], '*',
                    'compartmentId', compartmentId,
                    'context', decoded.context or '',
                    'ownerId', decoded.ownerId or '',
                    'taskType', decoded.taskType or '',
                    'maxRetries', tostring(decoded.maxRetries or 3),
                    'timeoutSeconds', tostring(decoded.timeoutSeconds or 300),
                    'createdAt', decoded.createdAt or '',
                    'payload', payload,
                    'data', jobData)
                  redis.call('DEL', ARGV[2] .. compartmentId)
                  redis.call('XDEL', KEYS[1], record[1])
                  table.insert(moved, compartmentId)
                end
              end
            end
            return moved
            """, List.class);

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
        Set<String> activeKeys = scanHeartbeatKeys();

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
            if (elapsed > STALE_REGISTRY_RETENTION_SECONDS) {
                workerRegistry.remove(entry.getKey(), existing);
                continue;
            }
            if (elapsed > HEARTBEAT_TIMEOUT_SECONDS && existing.isHealthy()) {
                existing.setHealthy(false);
                existing.setStatus("DEAD");
                existing.setSecondsSinceLastHeartbeat(elapsed);
                log.warn("Worker {} key expired or timed out ({}s elapsed) - flagged DEAD", entry.getKey(), elapsed);
            }
        }
    }

    Set<String> scanHeartbeatKeys() {
        try {
            Set<String> keys = redisTemplate.execute((RedisCallback<Set<String>>) connection -> {
                Set<String> matches = new HashSet<>();
                ScanOptions options = ScanOptions.scanOptions().match(heartbeatPattern).count(200).build();
                try (Cursor<byte[]> cursor = connection.scan(options)) {
                    while (cursor.hasNext()) {
                        String key = redisTemplate.getStringSerializer().deserialize(cursor.next());
                        if (key != null) {
                            matches.add(key);
                        }
                    }
                }
                return matches;
            });
            return keys != null ? keys : Collections.emptySet();
        } catch (Exception e) {
            log.debug("Redis heartbeat SCAN skipped or failed: {}", e.getMessage());
            return Collections.emptySet();
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
        try {
            List<String> redriven = (List<String>) redisTemplate.execute(
                    REDRIVE_SCRIPT, List.of(dlqStreamName, streamName),
                    String.valueOf(capped), "coldharbor:retries:");
            if (redriven == null) {
                redriven = List.of();
            }
            Map<String, Object> result = new java.util.LinkedHashMap<>();
            result.put("redriven", redriven.size());
            result.put("ids", redriven);
            return result;
        } catch (Exception e) {
            log.warn("DLQ redrive failed: {}", e.getMessage());
            throw new RuntimeException("DLQ redrive failed", e);
        }
    }
}
