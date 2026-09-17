package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.WorkerStatusResponse;
import com.coldharbor.controlplane.model.HeartbeatPayload;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.data.redis.core.script.DefaultRedisScript;
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Instant;
import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class WorkerWellnessMonitorTest {

    private StringRedisTemplate redisTemplate;
    private ValueOperations<String, String> valueOperations;
    private ObjectMapper objectMapper;
    private WorkerWellnessMonitor monitor;

    @BeforeEach
    void setUp() {
        redisTemplate = mock(StringRedisTemplate.class);
        valueOperations = mock(ValueOperations.class);
        when(redisTemplate.opsForValue()).thenReturn(valueOperations);

        objectMapper = new ObjectMapper();
        objectMapper.registerModule(new JavaTimeModule());

        monitor = spy(new WorkerWellnessMonitor(redisTemplate, objectMapper));
        ReflectionTestUtils.setField(monitor, "heartbeatPattern", "worker:*:heartbeat");
    }

    @Test
    @DisplayName("Scans active worker heartbeats and marks recent pulses as healthy")
    void shouldIdentifyHealthyWorkers() throws Exception {
        HeartbeatPayload hb = new HeartbeatPayload("worker-go-01", "BUSY", "cpt_99", Instant.now().minusSeconds(5));
        String json = objectMapper.writeValueAsString(hb);

        doReturn(Set.of("worker:worker-go-01:heartbeat")).when(monitor).scanHeartbeatKeys();
        when(valueOperations.get("worker:worker-go-01:heartbeat")).thenReturn(json);

        monitor.scanWorkerHeartbeats();
        List<WorkerStatusResponse> workers = monitor.getActiveWorkers();

        assertThat(workers).hasSize(1);
        WorkerStatusResponse w = workers.get(0);
        assertThat(w.getWorkerId()).isEqualTo("worker-go-01");
        assertThat(w.getStatus()).isEqualTo("BUSY");
        assertThat(w.isHealthy()).isTrue();
        assertThat(w.getActiveCompartmentId()).isEqualTo("cpt_99");
    }

    @Test
    @DisplayName("Flags worker as DEAD when heartbeat timestamp is older than 30 seconds")
    void shouldFlagDeadWorkerWhenStale() throws Exception {
        HeartbeatPayload staleHb = new HeartbeatPayload("worker-go-02", "BUSY", "cpt_88", Instant.now().minusSeconds(45));
        String json = objectMapper.writeValueAsString(staleHb);

        doReturn(Set.of("worker:worker-go-02:heartbeat")).when(monitor).scanHeartbeatKeys();
        when(valueOperations.get("worker:worker-go-02:heartbeat")).thenReturn(json);

        monitor.scanWorkerHeartbeats();
        List<WorkerStatusResponse> workers = monitor.getActiveWorkers();

        assertThat(workers).hasSize(1);
        WorkerStatusResponse w = workers.get(0);
        assertThat(w.getWorkerId()).isEqualTo("worker-go-02");
        assertThat(w.getStatus()).isEqualTo("DEAD");
        assertThat(w.isHealthy()).isFalse();
        assertThat(w.getSecondsSinceLastHeartbeat()).isGreaterThanOrEqualTo(45L);
    }

    @Test
    @DisplayName("Redrive moves DLQ entries back to the main stream and clears them")
    @SuppressWarnings({"unchecked", "rawtypes"})
    void shouldRedriveDlqEntries() throws Exception {
        ReflectionTestUtils.setField(monitor, "dlqStreamName", "coldharbor:jobs:dlq");
        ReflectionTestUtils.setField(monitor, "streamName", "coldharbor:jobs");

        when(redisTemplate.execute(any(DefaultRedisScript.class), anyList(), any(), any()))
                .thenReturn(java.util.List.of("cpt_dlq_1"));

        java.util.Map<String, Object> result = monitor.redriveDlq(10);

        assertThat(result.get("redriven")).isEqualTo(1);
        verify(redisTemplate).execute(any(DefaultRedisScript.class),
                eq(java.util.List.of("coldharbor:jobs:dlq", "coldharbor:jobs")),
                eq("10"), eq("coldharbor:retries:"));
    }
}
