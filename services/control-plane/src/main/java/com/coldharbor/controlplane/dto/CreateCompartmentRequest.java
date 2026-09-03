package com.coldharbor.controlplane.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Pattern;

import java.util.Map;

/**
 * Request body for creating and dispatching a new compartment job.
 */
public class CreateCompartmentRequest {

    private String compartmentId;

    @NotBlank(message = "context is required")
    @Pattern(regexp = "^(INNIE|OUTIE)$", message = "context must be either INNIE or OUTIE")
    private String context;

    @NotBlank(message = "ownerId is required")
    private String ownerId;

    @NotBlank(message = "taskType is required")
    private String taskType;

    private Map<String, Object> payload;
    private Integer maxRetries = 3;
    private Integer timeoutSeconds = 300;

    public CreateCompartmentRequest() {
    }

    public CreateCompartmentRequest(String compartmentId, String context, String ownerId,
                                    String taskType, Map<String, Object> payload) {
        this.compartmentId = compartmentId;
        this.context = context;
        this.ownerId = ownerId;
        this.taskType = taskType;
        this.payload = payload;
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
}
