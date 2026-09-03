package com.coldharbor.controlplane.dto;

import com.fasterxml.jackson.annotation.JsonFormat;

import java.time.Instant;

/**
 * Worker liveness and telemetry report.
 */
public class WorkerStatusResponse {

    private String workerId;
    private String status; // IDLE, BUSY, DEAD
    private String activeCompartmentId;

    @JsonFormat(shape = JsonFormat.Shape.STRING)
    private Instant lastHeartbeat;

    private boolean healthy;
    private long secondsSinceLastHeartbeat;

    public WorkerStatusResponse() {
    }

    public WorkerStatusResponse(String workerId, String status, String activeCompartmentId,
                                Instant lastHeartbeat, boolean healthy, long secondsSinceLastHeartbeat) {
        this.workerId = workerId;
        this.status = status;
        this.activeCompartmentId = activeCompartmentId;
        this.lastHeartbeat = lastHeartbeat;
        this.healthy = healthy;
        this.secondsSinceLastHeartbeat = secondsSinceLastHeartbeat;
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

    public Instant getLastHeartbeat() {
        return lastHeartbeat;
    }

    public void setLastHeartbeat(Instant lastHeartbeat) {
        this.lastHeartbeat = lastHeartbeat;
    }

    public boolean isHealthy() {
        return healthy;
    }

    public void setHealthy(boolean healthy) {
        this.healthy = healthy;
    }

    public long getSecondsSinceLastHeartbeat() {
        return secondsSinceLastHeartbeat;
    }

    public void setSecondsSinceLastHeartbeat(long secondsSinceLastHeartbeat) {
        this.secondsSinceLastHeartbeat = secondsSinceLastHeartbeat;
    }
}
