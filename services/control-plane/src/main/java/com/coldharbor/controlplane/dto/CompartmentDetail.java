package com.coldharbor.controlplane.dto;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonInclude;

import java.time.Instant;

/**
 * Detailed compartment state representation.
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public class CompartmentDetail {

    private String compartmentId;
    private String context;
    private String ownerId;
    private String taskType;
    private String state; // QUEUED, RUNNING, CHECKPOINT, COMPLETED, ARCHIVED, PURGED, FAILED
    private int progress;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant createdAt;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant updatedAt;

    private String details;

    public CompartmentDetail() {
    }

    public CompartmentDetail(String compartmentId, String context, String ownerId,
                             String taskType, String state, int progress,
                             Instant createdAt, Instant updatedAt, String details) {
        this.compartmentId = compartmentId;
        this.context = context;
        this.ownerId = ownerId;
        this.taskType = taskType;
        this.state = state;
        this.progress = progress;
        this.createdAt = createdAt;
        this.updatedAt = updatedAt;
        this.details = details;
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

    public String getState() {
        return state;
    }

    public void setState(String state) {
        this.state = state;
    }

    public int getProgress() {
        return progress;
    }

    public void setProgress(int progress) {
        this.progress = progress;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public void setCreatedAt(Instant createdAt) {
        this.createdAt = createdAt;
    }

    public Instant getUpdatedAt() {
        return updatedAt;
    }

    public void setUpdatedAt(Instant updatedAt) {
        this.updatedAt = updatedAt;
    }

    public String getDetails() {
        return details;
    }

    public void setDetails(String details) {
        this.details = details;
    }
}
