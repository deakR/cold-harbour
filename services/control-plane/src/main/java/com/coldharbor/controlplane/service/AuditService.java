package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.exception.NotFoundException;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.repository.AuditRecordRepository;
import com.coldharbor.controlplane.security.ContextClearance;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.persistence.criteria.Predicate;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.data.jpa.domain.Specification;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;

/**
 * Service managing PostgreSQL immutable audit records.
 */
@Service
public class AuditService {

    private static final Logger log = LoggerFactory.getLogger(AuditService.class);

    private final AuditRecordRepository auditRecordRepository;
    private final ObjectMapper objectMapper;

    public AuditService(AuditRecordRepository auditRecordRepository, ObjectMapper objectMapper) {
        this.auditRecordRepository = auditRecordRepository;
        this.objectMapper = objectMapper;
    }

    @Transactional
    public AuditRecord saveAuditRecord(AuditRecord record) {
        return auditRecordRepository.save(record);
    }

    @Transactional
    public synchronized AuditRecord recordCompletedJob(String compartmentId, DeadDropPayload deadDrop, String details) {
        if (auditRecordRepository.existsByCompartmentIdAndFinalState(compartmentId, "PURGED") ||
                auditRecordRepository.existsByCompartmentIdAndFinalState(compartmentId, "ARCHIVED") ||
                auditRecordRepository.existsByCompartmentIdAndFinalState(compartmentId, "COMPLETED")) {
            log.debug("Audit record for compartment {} already exists; skipping duplicate persistence", compartmentId);
            return auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc(compartmentId).orElse(null);
        }

        String metadataJson = "{}";
        try {
            metadataJson = objectMapper.writeValueAsString(Map.of(
                    "details", details != null ? details : "",
                    "output", deadDrop.getOutput() != null ? deadDrop.getOutput() : Map.of()
            ));
        } catch (Exception e) {
            log.warn("Failed to serialize audit metadata: {}", e.getMessage());
        }

        AuditRecord record = new AuditRecord(
                UUID.randomUUID(),
                compartmentId,
                deadDrop.getOwnerId() != null ? deadDrop.getOwnerId() : "unknown",
                deadDrop.getContext() != null ? deadDrop.getContext() : "INNIE",
                deadDrop.getTaskType() != null ? deadDrop.getTaskType() : "TASK",
                "PURGED",
                deadDrop.getChecksum() != null ? deadDrop.getChecksum() : "",
                deadDrop.getDurationMs() != null ? deadDrop.getDurationMs() : 0L,
                deadDrop.getArchivedAt() != null ? deadDrop.getArchivedAt() : Instant.now(),
                deadDrop.getCompletedAt() != null ? deadDrop.getCompletedAt() : Instant.now(),
                metadataJson
        );

        AuditRecord saved = auditRecordRepository.save(record);
        log.info("Persisted durable audit record {} for compartment {} [context: {}, checksum: {}]",
                saved.getId(), compartmentId, saved.getContext(), saved.getChecksum());
        return saved;
    }

    @Transactional(readOnly = true)
    public List<AuditRecord> findAudits(String compartmentId, String ownerId, String contextFilter,
                                        ContextClearance callerClearance) {
        // Enforce clearance rules
        String effectiveContext = contextFilter;
        if (callerClearance == ContextClearance.INNIE) {
            if (contextFilter != null && !contextFilter.equalsIgnoreCase("INNIE")) {
                throw new ForbiddenException("INNIE clearance cannot query non-INNIE audit records");
            }
            effectiveContext = "INNIE";
        } else if (callerClearance == ContextClearance.OUTIE) {
            if (contextFilter != null && !contextFilter.equalsIgnoreCase("OUTIE")) {
                throw new ForbiddenException("OUTIE clearance cannot query non-OUTIE audit records");
            }
            effectiveContext = "OUTIE";
        }

        final String finalEffectiveContext = effectiveContext;

        Specification<AuditRecord> spec = (root, query, cb) -> {
            List<Predicate> predicates = new ArrayList<>();
            if (compartmentId != null && !compartmentId.trim().isEmpty()) {
                predicates.add(cb.equal(root.get("compartmentId"), compartmentId.trim()));
            }
            if (ownerId != null && !ownerId.trim().isEmpty()) {
                predicates.add(cb.equal(root.get("ownerId"), ownerId.trim()));
            }
            if (finalEffectiveContext != null && !finalEffectiveContext.trim().isEmpty()) {
                predicates.add(cb.equal(root.get("context"), finalEffectiveContext.trim().toUpperCase()));
            }
            query.orderBy(cb.desc(root.get("createdAt")));
            return cb.and(predicates.toArray(new Predicate[0]));
        };

        return auditRecordRepository.findAll(spec);
    }

    @Transactional(readOnly = true)
    public AuditRecord findById(UUID id, ContextClearance callerClearance) {
        AuditRecord record = auditRecordRepository.findById(id)
                .orElseThrow(() -> new NotFoundException("Audit record not found with id: " + id));

        if (callerClearance != null && !callerClearance.canAccess(record.getContext())) {
            throw new ForbiddenException("Caller clearance " + callerClearance +
                    " cannot access audit record in context " + record.getContext());
        }

        return record;
    }
}
