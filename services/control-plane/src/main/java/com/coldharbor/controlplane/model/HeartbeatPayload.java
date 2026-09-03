package com.coldharbor.controlplane.model;

import com.fasterxml.jackson.annotation.JsonFormat;
import com.fasterxml.jackson.annotation.JsonIgnoreProperties;

import java.time.Instant;

/**
 * Heartbeat payload stored in worker:{workerId}:heartbeat with TTL.
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public class HeartbeatPayload {

    private String workerId;
    private String status = "IDLE"; // IDLE, BUSY
    private String activeCompartmentId;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant timestamp = Instant.now();

    public HeartbeatPayload() {
    }

    public HeartbeatPayload(String workerId, String status, String activeCompartmentId, Instant timestamp) {
        this.workerId = workerId;
        this.status = status;
        this.activeCompartmentId = activeCompartmentId;
        this.timestamp = timestamp != null ? timestamp : Instant.now();
    }

    public String getWorkerId() {
        return workerId;
    }

    public void setWorkerId(String workerId) {
        this.workerId = workerId;
    }

    public String getStatus() {
        return status;
    }

    public void setStatus(String status) {
        this.status = status;
    }

    public String getActiveCompartmentId() {
        return activeCompartmentId;
    }

    public void setActiveCompartmentId(String activeCompartmentId) {
        this.activeCompartmentId = activeCompartmentId;
    }

    public Instant getTimestamp() {
        return timestamp;
    }

    public void setTimestamp(Instant timestamp) {
        this.timestamp = timestamp;
    }
}
