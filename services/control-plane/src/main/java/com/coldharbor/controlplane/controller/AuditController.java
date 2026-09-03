package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.security.ClearanceContext;
import com.coldharbor.controlplane.security.ContextClearance;
import com.coldharbor.controlplane.service.AuditService;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;
import java.util.UUID;

/**
 * Controller querying immutable audit records from PostgreSQL.
 */
@RestController
@RequestMapping({"/api/v1/audits", "/api/audits"})
public class AuditController {

    private final AuditService auditService;

    public AuditController(AuditService auditService) {
        this.auditService = auditService;
    }

    /**
     * Queries historical audit records with optional filters.
     * Enforces context namespace isolation: INNIE callers only see INNIE audits, OUTIE callers only see OUTIE audits.
     */
    @GetMapping
    public ResponseEntity<List<AuditRecord>> getAudits(
            @RequestParam(name = "compartmentId", required = false) String compartmentId,
            @RequestParam(name = "ownerId", required = false) String ownerId,
            @RequestParam(name = "context", required = false) String context) {

        ContextClearance clearance = ClearanceContext.getClearance();
        List<AuditRecord> records = auditService.findAudits(compartmentId, ownerId, context, clearance);
        return ResponseEntity.ok(records);
    }

    /**
     * Queries an individual audit record by UUID.
     */
    @GetMapping("/{id}")
    public ResponseEntity<AuditRecord> getAuditById(@PathVariable("id") UUID id) {
        ContextClearance clearance = ClearanceContext.getClearance();
        AuditRecord record = auditService.findById(id, clearance);
        return ResponseEntity.ok(record);
    }
}
