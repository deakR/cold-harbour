package com.coldharbor.controlplane.dto;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;

import java.time.Instant;

/**
 * Verified Dead Drop response.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class DeadDropResponse {

    private String compartmentId;
    private String ownerId;
    private String context;
    private String taskType;
    private Object output;
    private String checksum;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant archivedAt;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant completedAt;

    private Long durationMs;
    private Integer ttlSeconds;
    private Long remainingTtlSeconds;

    public DeadDropResponse() {
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

    public Object getOutput() {
        return output;
    }

    public void setOutput(Object output) {
        this.output = output;
    }

    public String getChecksum() {
        return checksum;
    }

    public void setChecksum(String checksum) {
        this.checksum = checksum;
    }

    public Instant getArchivedAt() {
        return archivedAt;
    }

    public void setArchivedAt(Instant archivedAt) {
        this.archivedAt = archivedAt;
    }

    public Instant getCompletedAt() {
        return completedAt;
    }

    public void setCompletedAt(Instant completedAt) {
        this.completedAt = completedAt;
    }

    public Long getDurationMs() {
        return durationMs;
    }

    public void setDurationMs(Long durationMs) {
        this.durationMs = durationMs;
    }

    public Integer getTtlSeconds() {
        return ttlSeconds;
    }

    public void setTtlSeconds(Integer ttlSeconds) {
        this.ttlSeconds = ttlSeconds;
    }

    public Long getRemainingTtlSeconds() {
        return remainingTtlSeconds;
    }

    public void setRemainingTtlSeconds(Long remainingTtlSeconds) {
        this.remainingTtlSeconds = remainingTtlSeconds;
    }
}
