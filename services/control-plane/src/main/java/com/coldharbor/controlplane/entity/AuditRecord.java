package com.coldharbor.controlplane.entity;

import jakarta.persistence.*;
import java.time.Instant;
import java.util.UUID;

/**
 * Immutable execution audit record mapped to PostgreSQL audit_records table.
 */
@Entity
@Table(name = "audit_records", indexes = {
        @Index(name = "idx_audit_compartment_id", columnList = "compartment_id"),
        @Index(name = "idx_audit_owner_id", columnList = "owner_id"),
        @Index(name = "idx_audit_created_at", columnList = "created_at DESC"),
        @Index(name = "idx_audit_context", columnList = "context")
})
public class AuditRecord {

    @Id
    @Column(name = "id", nullable = false, updatable = false)
    private UUID id;

    @Column(name = "compartment_id", nullable = false, length = 64)
    private String compartmentId;

    @Column(name = "owner_id", nullable = false, length = 64)
    private String ownerId;

    @Column(name = "context", nullable = false, length = 16)
    private String context;

    @Column(name = "task_type", nullable = false, length = 64)
    private String taskType;

    @Column(name = "final_state", nullable = false, length = 32)
    private String finalState;

    @Column(name = "checksum", nullable = false, length = 64)
    private String checksum;

    @Column(name = "duration_ms", nullable = false)
    private Long durationMs = 0L;

    @Column(name = "created_at", nullable = false)
    private Instant createdAt;

    @Column(name = "completed_at", nullable = false)
    private Instant completedAt;

    @Column(name = "metadata", columnDefinition = "jsonb", nullable = false)
    private String metadata = "{}";

    public AuditRecord() {
    }

    public AuditRecord(UUID id, String compartmentId, String ownerId, String context,
                       String taskType, String finalState, String checksum,
                       Long durationMs, Instant createdAt, Instant completedAt, String metadata) {
        this.id = id != null ? id : UUID.randomUUID();
        this.compartmentId = compartmentId;
        this.ownerId = ownerId;
        this.context = context;
        this.taskType = taskType;
        this.finalState = finalState;
        this.checksum = checksum != null ? checksum : "";
        this.durationMs = durationMs != null ? durationMs : 0L;
        this.createdAt = createdAt != null ? createdAt : Instant.now();
        this.completedAt = completedAt != null ? completedAt : Instant.now();
        this.metadata = (metadata != null && !metadata.trim().isEmpty()) ? metadata : "{}";
    }

    @PrePersist
    protected void onCreate() {
        if (this.id == null) {
            this.id = UUID.randomUUID();
        }
        if (this.createdAt == null) {
            this.createdAt = Instant.now();
        }
        if (this.completedAt == null) {
            this.completedAt = Instant.now();
        }
        if (this.metadata == null || this.metadata.trim().isEmpty()) {
            this.metadata = "{}";
        }
        if (this.durationMs == null) {
            this.durationMs = 0L;
        }
        if (this.checksum == null) {
            this.checksum = "";
        }
    }

    public UUID getId() {
        return id;
    }

    public void setId(UUID id) {
        this.id = id;
    }

    public String getCompartmentId() {
        return compartmentId;
    }

    public void setCompartmentId(String compartmentId) {
        this.compartmentId = compartmentId;
    }

    public String getOwnerId() {
        return ownerId;
    }

    public void setOwnerId(String ownerId) {
        this.ownerId = ownerId;
    }

    public String getContext() {
        return context;
    }

    public void setContext(String context) {
        this.context = context;
    }

    public String getTaskType() {
        return taskType;
    }

    public void setTaskType(String taskType) {
        this.taskType = taskType;
    }

    public String getFinalState() {
        return finalState;
    }

    public void setFinalState(String finalState) {
        this.finalState = finalState;
    }

    public String getChecksum() {
        return checksum;
    }

    public void setChecksum(String checksum) {
        this.checksum = checksum;
    }

    public Long getDurationMs() {
        return durationMs;
    }

    public void setDurationMs(Long durationMs) {
        this.durationMs = durationMs;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(Instant createdAt) {
        this.createdAt = createdAt;
    }

    public Instant getCompletedAt() {
        return completedAt;
    }

    public void setCompletedAt(Instant completedAt) {
        this.completedAt = completedAt;
    }

    public String getMetadata() {
        return metadata;
    }

    public void setMetadata(String metadata) {
        this.metadata = metadata;
    }
}
