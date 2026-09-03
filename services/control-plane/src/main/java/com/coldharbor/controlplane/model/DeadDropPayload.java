package com.coldharbor.controlplane.model;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.time.Instant;
import java.util.Map;

/**
 * Sealed Dead Drop output stored in archive:{compartmentId} with TTL.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class DeadDropPayload {

    private String compartmentId;
    private String ownerId;
    private String context;
    private String taskType;
    private Map<String, Object> output;
    private Object resultPayload;
    private String checksum;
    private Long durationMs;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant archivedAt;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant completedAt;

    private Integer ttlSeconds;

    public DeadDropPayload() {
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

    public Map<String, Object> getOutput() {
        return output;
    }

    public void setOutput(Map<String, Object> output) {
        this.output = output;
    }

    public Object getResultPayload() {
        return resultPayload;
    }

    public void setResultPayload(Object resultPayload) {
        this.resultPayload = resultPayload;
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

    public Integer getTtlSeconds() {
        return ttlSeconds;
    }

    public void setTtlSeconds(Integer ttlSeconds) {
        this.ttlSeconds = ttlSeconds;
    }
}
