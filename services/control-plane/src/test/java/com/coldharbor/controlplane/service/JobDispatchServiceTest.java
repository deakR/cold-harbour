package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.CreateCompartmentRequest;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Duration;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class JobDispatchServiceTest {

    private StringRedisTemplate redisTemplate;
    private ValueOperations<String, String> valueOperations;
    private ObjectMapper objectMapper;
    private JobDispatchService dispatchService;

    @BeforeEach
    void setUp() {
        redisTemplate = mock(StringRedisTemplate.class);
        valueOperations = mock(ValueOperations.class);

        when(redisTemplate.opsForValue()).thenReturn(valueOperations);

        objectMapper = new ObjectMapper();
        objectMapper.registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule());
        dispatchService = new JobDispatchService(redisTemplate, objectMapper);
        ReflectionTestUtils.setField(dispatchService, "streamName", "coldharbor:jobs");
        ReflectionTestUtils.setField(dispatchService, "metaPrefix", "compartment:");
    }

    @Test
    @DisplayName("Dispatches job to Redis stream coldharbor:jobs and sets QUEUED metadata")
    void shouldDispatchJobToStream() {
        CreateCompartmentRequest req = new CreateCompartmentRequest();
        req.setCompartmentId("cpt_abc123");
        req.setContext("INNIE");
        req.setOwnerId("usr_9918");
        req.setTaskType("DATA_REDUCTION");
        req.setPayload(Map.of("batchSize", 500));

        when(redisTemplate.execute(any(DefaultRedisScript.class), anyList(), any(String[].class)))
                .thenReturn(1L);

        CompartmentDetail result = dispatchService.dispatchJob(req);

        assertThat(result).isNotNull();
        assertThat(result.getCompartmentId()).isEqualTo("cpt_abc123");
        assertThat(result.getState()).isEqualTo("QUEUED");
        assertThat(result.getProgress()).isEqualTo(0);

        verify(redisTemplate).execute(any(DefaultRedisScript.class),
                eq(java.util.List.of("compartment:cpt_abc123:meta", "coldharbor:jobs",
                        "compartment:cpt_abc123:meta:dispatch")),
                any(String[].class));
        verify(valueOperations, never()).set(anyString(), anyString(), any(Duration.class));
    }

    @Test
    void shouldRejectDuplicateCompartmentId() {
        CreateCompartmentRequest req = new CreateCompartmentRequest(
                "cpt_duplicate", "INNIE", "usr_1", "TASK", Map.of());
        when(redisTemplate.execute(any(DefaultRedisScript.class), anyList(), any(String[].class)))
                .thenReturn(0L);

        org.assertj.core.api.Assertions.assertThatThrownBy(() -> dispatchService.dispatchJob(req))
                .isInstanceOf(IllegalArgumentException.class)
                .hasMessageContaining("already exists");
    }
}
