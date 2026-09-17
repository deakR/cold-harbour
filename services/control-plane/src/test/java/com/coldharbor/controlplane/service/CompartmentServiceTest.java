package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.DeadDropResponse;
import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.exception.ForbiddenException;
import com.coldharbor.controlplane.exception.NotFoundException;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.repository.AuditRecordRepository;
import com.coldharbor.controlplane.security.ContextClearance;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;
import java.util.UUID;
import java.util.concurrent.TimeUnit;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.*;

class CompartmentServiceTest {

    private StringRedisTemplate redisTemplate;
    private ValueOperations<String, String> valueOperations;
    private AuditRecordRepository auditRecordRepository;
    private CompartmentService compartmentService;
    private ObjectMapper objectMapper;

    @BeforeEach
    void setUp() {
        redisTemplate = mock(StringRedisTemplate.class);
        valueOperations = mock(ValueOperations.class);
        when(redisTemplate.opsForValue()).thenReturn(valueOperations);
        auditRecordRepository = mock(AuditRecordRepository.class);
        objectMapper = new ObjectMapper();
        objectMapper.registerModule(new JavaTimeModule());
        compartmentService = new CompartmentService(redisTemplate, auditRecordRepository, objectMapper);
        ReflectionTestUtils.setField(compartmentService, "metaPrefix", "compartment:");
        ReflectionTestUtils.setField(compartmentService, "archivePrefix", "archive:");
    }

    private AuditRecord audit(String compartmentId) {
        Map<String, Object> output = new java.util.LinkedHashMap<>();
        output.put("processedCount", 200);
        output.put("status", "SUCCESS");
        String checksum = checksum(output);
        String metadata;
        try {
            metadata = objectMapper.writeValueAsString(Map.of("output", output));
        } catch (Exception e) {
            throw new AssertionError(e);
        }
        return new AuditRecord(
                UUID.randomUUID(), compartmentId, "usr_1", "INNIE", "DATA_REDUCTION",
                "PURGED", checksum, 9L, Instant.now(), Instant.now(),
                metadata);
    }

    private String checksum(Object value) {
        try {
            return java.util.HexFormat.of().formatHex(java.security.MessageDigest.getInstance("SHA-256")
                    .digest(objectMapper.writeValueAsBytes(value)));
        } catch (Exception e) {
            throw new AssertionError(e);
        }
    }

    @Test
    @DisplayName("Live archive key is served with remaining TTL")
    void shouldServeLiveDeadDrop() throws Exception {
        DeadDropPayload payload = new DeadDropPayload();
        payload.setCompartmentId("cpt_live");
        payload.setOwnerId("usr_1");
        payload.setContext("INNIE");
        Map<String, Object> output = Map.of("processedCount", 200);
        payload.setChecksum(checksum(output));
        payload.setOutput(output);
        when(valueOperations.get("archive:cpt_live")).thenReturn(objectMapper.writeValueAsString(payload));
        when(redisTemplate.getExpire("archive:cpt_live", TimeUnit.SECONDS)).thenReturn(3500L);

        DeadDropResponse response = compartmentService.getDeadDrop("cpt_live", ContextClearance.INNIE);

        assertThat(response.getChecksum()).isEqualTo(checksum(output));
        assertThat(response.getRemainingTtlSeconds()).isEqualTo(3500L);
    }

    @Test
    @DisplayName("Expired archive falls back to durable audit record with zero TTL")
    void shouldFallBackToAuditAfterExpiry() {
        when(valueOperations.get("archive:cpt_old")).thenReturn(null);
        when(auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc("cpt_old"))
                .thenReturn(Optional.of(audit("cpt_old")));

        DeadDropResponse response = compartmentService.getDeadDrop("cpt_old", ContextClearance.SYSTEM);

        assertThat(response.getChecksum()).isNotBlank();
        assertThat(response.getRemainingTtlSeconds()).isEqualTo(0L);
        assertThat(response.getOutput()).isNotNull();
    }

    @Test
    @DisplayName("Missing archive and audit raises NotFound")
    void shouldRaiseNotFoundWhenBothMissing() {
        when(valueOperations.get("archive:cpt_gone")).thenReturn(null);
        when(auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc("cpt_gone"))
                .thenReturn(Optional.empty());

        assertThatThrownBy(() -> compartmentService.getDeadDrop("cpt_gone", ContextClearance.ADMIN))
                .isInstanceOf(NotFoundException.class);
    }

    @Test
    @DisplayName("OUTIE caller cannot read INNIE fallback record")
    void shouldEnforceClearanceOnFallback() {
        when(valueOperations.get("archive:cpt_old")).thenReturn(null);
        when(auditRecordRepository.findFirstByCompartmentIdOrderByCompletedAtDesc("cpt_old"))
                .thenReturn(Optional.of(audit("cpt_old")));

        assertThatThrownBy(() -> compartmentService.getDeadDrop("cpt_old", ContextClearance.OUTIE))
                .isInstanceOf(ForbiddenException.class);
    }

    @Test
    void shouldRejectTamperedDeadDrop() throws Exception {
        DeadDropPayload payload = new DeadDropPayload();
        payload.setCompartmentId("cpt_tampered");
        payload.setContext("INNIE");
        payload.setChecksum(checksum(Map.of("value", 1)));
        payload.setOutput(Map.of("value", 2));
        when(valueOperations.get("archive:cpt_tampered"))
                .thenReturn(objectMapper.writeValueAsString(payload));

        assertThatThrownBy(() -> compartmentService.getDeadDrop("cpt_tampered", ContextClearance.INNIE))
                .isInstanceOf(IllegalStateException.class)
                .hasMessage("Dead drop integrity verification failed");
    }
}
