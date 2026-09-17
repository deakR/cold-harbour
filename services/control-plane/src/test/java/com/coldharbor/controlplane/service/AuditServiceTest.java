package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.repository.AuditRecordRepository;
import com.coldharbor.controlplane.security.ContextClearance;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.data.jpa.domain.Specification;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

class AuditServiceTest {

    private AuditRecordRepository repository;
    private ObjectMapper objectMapper;
    private AuditService auditService;

    @BeforeEach
    void setUp() {
        repository = mock(AuditRecordRepository.class);
        objectMapper = new ObjectMapper();
        objectMapper.registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule());
        auditService = new AuditService(repository, objectMapper);
    }

    @Test
    @DisplayName("Successfully creates and persists an audit record from Dead Drop payload")
    void shouldPersistAuditFromDeadDrop() {
        DeadDropPayload deadDrop = new DeadDropPayload();
        deadDrop.setCompartmentId("cpt_123");
        deadDrop.setOwnerId("usr_999");
        deadDrop.setContext("INNIE");
        deadDrop.setTaskType("DATA_REDUCTION");
        deadDrop.setChecksum("sha256_mock_hash");
        deadDrop.setDurationMs(320L);
        deadDrop.setOutput(Map.of("processedCount", 500));
        deadDrop.setArchivedAt(Instant.now());

        when(repository.existsByCompartmentIdAndFinalState("cpt_123", "PURGED")).thenReturn(false);
        when(repository.saveAndFlush(any(AuditRecord.class))).thenAnswer(i -> i.getArgument(0));

        AuditRecord record = auditService.recordCompletedJob("cpt_123", deadDrop, "Completed successfully");

        assertThat(record).isNotNull();
        assertThat(record.getCompartmentId()).isEqualTo("cpt_123");
        assertThat(record.getContext()).isEqualTo("INNIE");
        assertThat(record.getFinalState()).isEqualTo("PURGED");
        assertThat(record.getChecksum()).isEqualTo("sha256_mock_hash");
        assertThat(record.getDurationMs()).isEqualTo(320L);

        verify(repository, times(1)).saveAndFlush(any(AuditRecord.class));
    }

    @Test
    @DisplayName("Prevents duplicate audit records if compartment was already archived or purged")
    void shouldPreventDuplicateAuditRecords() {
        AuditRecord existing = new AuditRecord(UUID.randomUUID(), "cpt_dup", "usr_1", "INNIE", "TASK", "PURGED", "c1", 100L, Instant.now(), Instant.now(), "{}");
        when(repository.findFirstByCompartmentIdAndFinalStateOrderByCompletedAtDesc(
                "cpt_dup", "PURGED")).thenReturn(Optional.of(existing));
        when(repository.saveAndFlush(any(AuditRecord.class)))
                .thenThrow(new org.springframework.dao.DataIntegrityViolationException("duplicate"));

        DeadDropPayload deadDrop = new DeadDropPayload();
        deadDrop.setCompartmentId("cpt_dup");
        deadDrop.setOwnerId("usr_1");
        deadDrop.setContext("INNIE");
        deadDrop.setTaskType("TASK");

        AuditRecord result = auditService.recordCompletedJob("cpt_dup", deadDrop, "details");
        assertThat(result).isNotNull();
        assertThat(result.getCompartmentId()).isEqualTo("cpt_dup");
        verify(repository).saveAndFlush(any(AuditRecord.class));
    }

    @Test
    @DisplayName("INNIE clearance can only query INNIE audit records and rejects OUTIE query")
    void shouldEnforceInnieClearanceQueryRules() {
        when(repository.findAll(any(Specification.class))).thenReturn(List.of());

        // Valid INNIE query
        List<AuditRecord> list = auditService.findAudits("cpt_1", "usr_1", "INNIE", ContextClearance.INNIE);
        assertThat(list).isNotNull();

        // Attempting to query OUTIE while holding INNIE clearance throws ForbiddenException
        assertThatThrownBy(() -> auditService.findAudits("cpt_1", "usr_1", "OUTIE", ContextClearance.INNIE))
                .isInstanceOf(ForbiddenException.class)
                .hasMessageContaining("INNIE clearance cannot query non-INNIE audit records");
    }

    @Test
    @DisplayName("OUTIE clearance can only query OUTIE audit records and rejects INNIE query")
    void shouldEnforceOutieClearanceQueryRules() {
        when(repository.findAll(any(Specification.class))).thenReturn(List.of());

        // Valid OUTIE query
        List<AuditRecord> list = auditService.findAudits("cpt_2", "usr_2", "OUTIE", ContextClearance.OUTIE);
        assertThat(list).isNotNull();

        // Attempting to query INNIE while holding OUTIE clearance throws ForbiddenException
        assertThatThrownBy(() -> auditService.findAudits("cpt_2", "usr_2", "INNIE", ContextClearance.OUTIE))
                .isInstanceOf(ForbiddenException.class)
                .hasMessageContaining("OUTIE clearance cannot query non-OUTIE audit records");
    }

    @Test
    @DisplayName("SYSTEM and ADMIN clearance can query all contexts")
    void shouldAllowAdminAndSystemToQueryAll() {
        when(repository.findAll(any(Specification.class))).thenReturn(List.of());

        List<AuditRecord> adminList = auditService.findAudits(null, null, null, ContextClearance.ADMIN);
        assertThat(adminList).isNotNull();

        List<AuditRecord> systemList = auditService.findAudits(null, null, "INNIE", ContextClearance.SYSTEM);
        assertThat(systemList).isNotNull();
    }
}
