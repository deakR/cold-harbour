package com.coldharbor.controlplane.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.Max;
import jakarta.validation.constraints.Min;
import jakarta.validation.constraints.Pattern;
import jakarta.validation.constraints.Size;

import java.util.Map;

/**
 * Request body for creating and dispatching a new compartment job.
 */
public class CreateCompartmentRequest {

    @Pattern(regexp = "^[A-Za-z0-9_-]{1,64}$",
            message = "compartmentId must be 1-64 letters, numbers, underscores, or hyphens")
    private String compartmentId;

    @NotBlank(message = "context is required")
    @Pattern(regexp = "^(INNIE|OUTIE)$", message = "context must be either INNIE or OUTIE")
    private String context;

    @NotBlank(message = "ownerId is required")
    @Size(max = 64, message = "ownerId must not exceed 64 characters")
    @Pattern(regexp = "^[A-Za-z0-9_.@-]+$", message = "ownerId contains invalid characters")
    private String ownerId;

    @NotBlank(message = "taskType is required")
    @Size(max = 64, message = "taskType must not exceed 64 characters")
    @Pattern(regexp = "^[A-Za-z][A-Za-z0-9_-]*$", message = "taskType contains invalid characters")
    private String taskType;

    private Map<String, Object> payload;
    @Min(value = 0, message = "maxRetries must be at least 0")
    @Max(value = 20, message = "maxRetries must not exceed 20")
    private Integer maxRetries = 3;
    @Min(value = 1, message = "timeoutSeconds must be at least 1")
    @Max(value = 86400, message = "timeoutSeconds must not exceed 86400")
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
