package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.DeadDropResponse;
import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.exception.NotFoundException;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.repository.AuditRecordRepository;
import com.coldharbor.controlplane.security.ContextClearance;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.util.Optional;
import java.util.concurrent.TimeUnit;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;

/**
 * Service for querying compartment state and inspecting sealed Dead Drop archives.
 */
@Service
public class CompartmentService {

    private static final Logger log = LoggerFactory.getLogger(CompartmentService.class);

    private final StringRedisTemplate redisTemplate;
    private final AuditRecordRepository auditRecordRepository;
    private final ObjectMapper objectMapper;

    @Value("${coldharbor.redis.meta-prefix:compartment:}")
    private String metaPrefix;

    @Value("${coldharbor.redis.archive-prefix:archive:}")
    private String archivePrefix;

    public CompartmentService(StringRedisTemplate redisTemplate,
                              AuditRecordRepository auditRecordRepository,
                              ObjectMapper objectMapper) {
        this.redisTemplate = redisTemplate;
        this.auditRecordRepository = auditRecordRepository;
        this.objectMapper = objectMapper;
    }

    /**
     * Queries compartment metadata from Redis cache or durable PostgreSQL audit record.
     */
    public CompartmentDetail getCompartment(String compartmentId, ContextClearance callerClearance) {
        // 1. Try Redis live meta
        String metaKey = metaPrefix + compartmentId + ":meta";
        String metaJson = redisTemplate.opsForValue().get(metaKey);

        CompartmentDetail detail = null;
        if (metaJson != null) {
            try {
                detail = objectMapper.readValue(metaJson, CompartmentDetail.class);
            } catch (Exception e) {
                log.warn("Failed to parse compartment meta JSON: {}", e.getMessage());
            }
        }

        // 2. Fallback to PostgreSQL audit record
        if (detail == null) {
            Optional<AuditRecord> auditOpt = auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc(compartmentId);
            if (auditOpt.isPresent()) {
                AuditRecord ar = auditOpt.get();
                detail = new CompartmentDetail(
                        ar.getCompartmentId(),
                        ar.getContext(),
                        ar.getOwnerId(),
                        ar.getTaskType(),
                        ar.getFinalState(),
                        100,
                        ar.getCreatedAt(),
                        ar.getCompletedAt(),
                        "Terminal state " + ar.getFinalState() + ". Stored in durable audit record."
                );
            }
        }

        if (detail == null) {
            throw new NotFoundException("Compartment not found: " + compartmentId);
        }

        // 3. Enforce context isolation
        if (callerClearance != null && !callerClearance.canAccess(detail.getContext())) {
            throw new ForbiddenException("Clearance " + callerClearance + " cannot access " +
                    detail.getContext() + " compartment " + compartmentId);
        }

        return detail;
    }

    /**
     * Retrieves and verifies sealed Dead Drop archive.
     * Falls back to the durable PostgreSQL audit record when the Redis TTL has expired,
     * so results remain retrievable after the self-destruct window.
     */
    public DeadDropResponse getDeadDrop(String compartmentId, ContextClearance callerClearance) {
        String archiveKey = archivePrefix + compartmentId;
        String archiveJson = redisTemplate.opsForValue().get(archiveKey);

        if (archiveJson != null && !archiveJson.trim().isEmpty()) {

        DeadDropPayload payload;
        try {
            payload = objectMapper.readValue(archiveJson, DeadDropPayload.class);
        } catch (Exception e) {
            log.error("Failed to parse dead drop JSON for compartment {}: {}", compartmentId, e.getMessage());
            throw new IllegalStateException("Dead drop integrity verification failed", e);
        }

        // Enforce context clearance
        if (callerClearance != null && payload.getContext() != null && !callerClearance.canAccess(payload.getContext())) {
            throw new ForbiddenException("Clearance " + callerClearance + " cannot access " +
                    payload.getContext() + " dead drop for " + compartmentId);
        }

        verifyChecksum(payload.getOutput() != null ? payload.getOutput() : payload.getResultPayload(),
                payload.getChecksum());

        Long remainingTtl = redisTemplate.getExpire(archiveKey, TimeUnit.SECONDS);

        DeadDropResponse response = new DeadDropResponse();
        response.setCompartmentId(payload.getCompartmentId());
        response.setOwnerId(payload.getOwnerId());
        response.setContext(payload.getContext());
        response.setTaskType(payload.getTaskType());
        response.setOutput(payload.getOutput() != null ? payload.getOutput() : payload.getResultPayload());
        response.setChecksum(payload.getChecksum());
        response.setDurationMs(payload.getDurationMs());
        response.setArchivedAt(payload.getArchivedAt());
        response.setCompletedAt(payload.getCompletedAt() != null ? payload.getCompletedAt() : payload.getArchivedAt());
        response.setTtlSeconds(payload.getTtlSeconds());
        response.setRemainingTtlSeconds(remainingTtl != null && remainingTtl > 0 ? remainingTtl : 0L);

        return response;
        }

        return getDeadDropFromAudit(compartmentId, callerClearance);
    }

    private void verifyChecksum(Object target, String expected) {
        try {
            byte[] bytes;
            if (target == null) {
                bytes = new byte[0];
            } else if (target instanceof String string) {
                bytes = string.getBytes(StandardCharsets.UTF_8);
            } else if (target instanceof byte[] raw) {
                bytes = raw;
            } else {
                // Go's encoding/json escapes these characters by default.
                String json = objectMapper.writeValueAsString(target)
                        .replace("&", "\\u0026")
                        .replace("<", "\\u003c")
                        .replace(">", "\\u003e")
                        .replace("\u2028", "\\u2028")
                        .replace("\u2029", "\\u2029");
                bytes = json.getBytes(StandardCharsets.UTF_8);
            }
            String actual = HexFormat.of().formatHex(
                    MessageDigest.getInstance("SHA-256").digest(bytes));
            if (expected == null || !MessageDigest.isEqual(
                    actual.getBytes(StandardCharsets.US_ASCII),
                    expected.toLowerCase(java.util.Locale.ROOT).getBytes(StandardCharsets.US_ASCII))) {
                throw new IllegalStateException("Dead drop integrity verification failed");
            }
        } catch (IllegalStateException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalStateException("Dead drop integrity verification failed", e);
        }
    }

    private DeadDropResponse getDeadDropFromAudit(String compartmentId, ContextClearance callerClearance) {
        AuditRecord ar = auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc(compartmentId)
                .orElseThrow(() -> new NotFoundException("Dead drop not found or expired for compartment: " + compartmentId));
        if (callerClearance != null && !callerClearance.canAccess(ar.getContext())) {
            throw new ForbiddenException("Clearance " + callerClearance + " cannot access " +
                    ar.getContext() + " dead drop for " + compartmentId);
        }
        Object output = null;
        try {
            var node = objectMapper.readTree(ar.getMetadata());
            if (node != null && node.has("output")) {
                output = objectMapper.convertValue(node.get("output"), Object.class);
            }
        } catch (Exception e) {
            log.warn("Failed to parse audit metadata output for {}: {}", compartmentId, e.getMessage());
        }
        verifyChecksum(output, ar.getChecksum());
        DeadDropResponse response = new DeadDropResponse();
        response.setCompartmentId(ar.getCompartmentId());
        response.setOwnerId(ar.getOwnerId());
        response.setContext(ar.getContext());
        response.setTaskType(ar.getTaskType());
        response.setOutput(output);
        response.setChecksum(ar.getChecksum());
        response.setDurationMs(ar.getDurationMs());
        response.setArchivedAt(ar.getCompletedAt());
        response.setCompletedAt(ar.getCompletedAt());
        response.setRemainingTtlSeconds(0L);
        return response;
    }
}
