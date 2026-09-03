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
                        "Completed and purged. Stored in durable audit record."
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
     */
    public DeadDropResponse getDeadDrop(String compartmentId, ContextClearance callerClearance) {
        String archiveKey = archivePrefix + compartmentId;
        String archiveJson = redisTemplate.opsForValue().get(archiveKey);

        if (archiveJson == null || archiveJson.trim().isEmpty()) {
            throw new NotFoundException("Dead drop not found or expired for compartment: " + compartmentId);
        }

        DeadDropPayload payload;
        try {
            payload = objectMapper.readValue(archiveJson, DeadDropPayload.class);
        } catch (Exception e) {
            log.error("Failed to parse dead drop JSON for compartment {}: {}", compartmentId, e.getMessage());
            throw new RuntimeException("Corrupted dead drop payload: " + e.getMessage(), e);
        }

        // Enforce context clearance
        if (callerClearance != null && payload.getContext() != null && !callerClearance.canAccess(payload.getContext())) {
            throw new ForbiddenException("Clearance " + callerClearance + " cannot access " +
                    payload.getContext() + " dead drop for " + compartmentId);
        }

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
}
