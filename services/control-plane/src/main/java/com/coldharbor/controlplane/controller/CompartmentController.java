package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.CreateCompartmentRequest;
import com.coldharbor.controlplane.dto.DeadDropResponse;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.security.ClearanceContext;
import com.coldharbor.controlplane.security.ContextClearance;
import com.coldharbor.controlplane.service.CompartmentService;
import com.coldharbor.controlplane.service.JobDispatchService;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

/**
 * Controller exposing compartment creation, status inspection, and dead drop retrieval.
 */
@RestController
@RequestMapping({"/api/v1/compartments", "/api/compartments"})
public class CompartmentController {

    private static final Logger log = LoggerFactory.getLogger(CompartmentController.class);

    private final JobDispatchService jobDispatchService;
    private final CompartmentService compartmentService;

    public CompartmentController(JobDispatchService jobDispatchService, CompartmentService compartmentService) {
        this.jobDispatchService = jobDispatchService;
        this.compartmentService = compartmentService;
    }

    /**
     * Dispatches a new task compartment to Redis Stream coldharbor:jobs.
     */
    @PostMapping
    public ResponseEntity<CompartmentDetail> createCompartment(@Valid @RequestBody CreateCompartmentRequest request) {
        ContextClearance clearance = ClearanceContext.getClearance();
        if (clearance != null) {
            if (!clearance.canAccess(request.getContext())) {
                throw new ForbiddenException("Clearance " + clearance +
                        " is not authorized to create " + request.getContext() + " compartment");
            }
        }

        CompartmentDetail detail = jobDispatchService.dispatchJob(request);
        log.info("Created and dispatched compartment {} in context {}", detail.getCompartmentId(), detail.getContext());
        return ResponseEntity.status(HttpStatus.CREATED).body(detail);
    }

    /**
     * Queries current lifecycle state and progress for a compartment.
     */
    @GetMapping("/{id}")
    public ResponseEntity<CompartmentDetail> getCompartment(@PathVariable("id") String id) {
        ContextClearance clearance = ClearanceContext.getClearance();
        CompartmentDetail detail = compartmentService.getCompartment(id, clearance);
        return ResponseEntity.ok(detail);
    }

    /**
     * Retrieves the sealed Dead Drop archive output and SHA-256 seal.
     */
    @GetMapping("/{id}/deaddrop")
    public ResponseEntity<DeadDropResponse> getDeadDrop(@PathVariable("id") String id) {
        ContextClearance clearance = ClearanceContext.getClearance();
        DeadDropResponse response = compartmentService.getDeadDrop(id, clearance);
        return ResponseEntity.ok(response);
    }
}
