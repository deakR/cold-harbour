package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.dto.WorkerStatusResponse;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.security.ClearanceContext;
import com.coldharbor.controlplane.security.ContextClearance;
import com.coldharbor.controlplane.service.WorkerWellnessMonitor;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;

import java.util.List;
import java.util.Map;

/**
 * Controller exposing real-time worker fleet liveness and telemetry.
 */
@RestController
@RequestMapping({"/api/v1/workers", "/api/workers"})
public class WorkerController {

    private final WorkerWellnessMonitor workerWellnessMonitor;

    public WorkerController(WorkerWellnessMonitor workerWellnessMonitor) {
        this.workerWellnessMonitor = workerWellnessMonitor;
    }

    /**
     * Lists active workers, liveness status, and active compartments.
     */
    @GetMapping
    public ResponseEntity<List<WorkerStatusResponse>> getWorkers() {
        List<WorkerStatusResponse> workers = workerWellnessMonitor.getActiveWorkers();
        return ResponseEntity.ok(workers);
    }

    /**
     * Inspects dead-letter queue depth and most recent poison-pill entries.
     */
    @GetMapping("/dlq")
    public ResponseEntity<Map<String, Object>> getDlq(
            @RequestParam(name = "limit", defaultValue = "20") int limit) {
        requireOperatorClearance();
        return ResponseEntity.ok(workerWellnessMonitor.getDlqSnapshot(limit));
    }

    /**
     * Requeues dead-letter entries onto the main stream for another attempt.
     */
    @PostMapping("/dlq/redrive")
    public ResponseEntity<Map<String, Object>> redriveDlq(
            @RequestParam(name = "limit", defaultValue = "10") int limit) {
        requireOperatorClearance();
        return ResponseEntity.ok(workerWellnessMonitor.redriveDlq(limit));
    }

    private static void requireOperatorClearance() {
        ContextClearance clearance = ClearanceContext.getClearance();
        if (clearance != ContextClearance.ADMIN && clearance != ContextClearance.SYSTEM) {
            throw new ForbiddenException("ADMIN or SYSTEM clearance is required for DLQ operations");
        }
    }
}
