package com.coldharbor.controlplane.model;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.time.Instant;

/**
 * State transition event published to Redis Pub/Sub coldharbor:events.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class EventMessage {

    private String eventId;
    private String compartmentId;
    private String workerId;
    private String fromState;
    private String toState;
    private String previousState;
    private String currentState;
    private int checkpointPct;
    private int progress;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant timestamp = Instant.now();

    private String details;
    private String context;
    private String ownerId;
    private String taskType;

    public EventMessage() {
    }

    public String getEffectiveToState() {
        if (toState != null && !toState.isEmpty()) {
            return toState;
        }
        return currentState;
    }

    public String getEffectiveFromState() {
        if (fromState != null && !fromState.isEmpty()) {
            return fromState;
        }
        return previousState;
    }

    public int getEffectiveProgress() {
        if (progress > 0) {
            return progress;
        }
        return checkpointPct;
    }

    public String getEventId() {
        return eventId;
    }

    public void setEventId(String eventId) {
        this.eventId = eventId;
    }

    public String getCompartmentId() {
        return compartmentId;
    }

    public void setCompartmentId(String compartmentId) {
        this.compartmentId = compartmentId;
    }

    public String getWorkerId() {
        return workerId;
    }

    public void setWorkerId(String workerId) {
        this.workerId = workerId;
    }

    public String getFromState() {
        return fromState;
    }

    public void setFromState(String fromState) {
        this.fromState = fromState;
    }

    public String getToState() {
        return toState;
    }

    public void setToState(String toState) {
        this.toState = toState;
    }

    public String getPreviousState() {
        return previousState;
    }

    public void setPreviousState(String previousState) {
        this.previousState = previousState;
    }

    public String getCurrentState() {
        return currentState;
    }

    public void setCurrentState(String currentState) {
        this.currentState = currentState;
    }

    public int getCheckpointPct() {
        return checkpointPct;
    }

    public void setCheckpointPct(int checkpointPct) {
        this.checkpointPct = checkpointPct;
    }

    public int getProgress() {
        return progress;
    }

    public void setProgress(int progress) {
        this.progress = progress;
    }

    public Instant getTimestamp() {
        return timestamp;
    }

    public void setTimestamp(Instant timestamp) {
        this.timestamp = timestamp;
    }

    public String getDetails() {
        return details;
    }

    public void setDetails(String details) {
        this.details = details;
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
}
