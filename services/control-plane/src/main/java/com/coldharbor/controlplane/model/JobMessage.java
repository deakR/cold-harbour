package com.coldharbor.controlplane.model;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;

import java.time.Instant;
import java.util.Map;

/**
 * Structured job payload dispatched onto Redis Stream coldharbor:jobs.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class JobMessage {

    private String compartmentId;
    private String context;
    private String ownerId;
    private String taskType;
    private Map<String, Object> payload;
    private Integer maxRetries = 3;
    private Integer timeoutSeconds = 300;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant createdAt = Instant.now();

    public JobMessage() {
    }

    public JobMessage(String compartmentId, String context, String ownerId, String taskType,
                      Map<String, Object> payload, Integer maxRetries, Integer timeoutSeconds, Instant createdAt) {
        this.compartmentId = compartmentId;
        this.context = context;
        this.ownerId = ownerId;
        this.taskType = taskType;
        this.payload = payload;
        this.maxRetries = maxRetries != null ? maxRetries : 3;
        this.timeoutSeconds = timeoutSeconds != null ? timeoutSeconds : 300;
        this.createdAt = createdAt != null ? createdAt : Instant.now();
    }

    public String getCompartmentId() {
        return compartmentId;
    }

    public void setCompartmentId(String compartmentId) {
        this.compartmentId = compartmentId;
    }

    public String getContext() {
        return context;
    }

    public void setContext(String context) {
        this.context = context;
    }

    public String getOwnerId() {
        return ownerId;
    }

    public void setOwnerId(String ownerId) {
        this.ownerId = ownerId;
    }

    public String getTaskType() {
        return taskType;
    }

    public void setTaskType(String taskType) {
        this.taskType = taskType;
    }

    public Map<String, Object> getPayload() {
        return payload;
    }

    public void setPayload(Map<String, Object> payload) {
        this.payload = payload;
    }

    public Integer getMaxRetries() {
        return maxRetries;
    }

    public void setMaxRetries(Integer maxRetries) {
        this.maxRetries = maxRetries;
    }

    public Integer getTimeoutSeconds() {
        return timeoutSeconds;
    }

    public void setTimeoutSeconds(Integer timeoutSeconds) {
        this.timeoutSeconds = timeoutSeconds;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(Instant createdAt) {
        this.createdAt = createdAt;
    }
}
