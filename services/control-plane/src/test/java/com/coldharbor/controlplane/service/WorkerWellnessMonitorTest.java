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
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Instant;
import java.util.List;
import java.util.Set;

import static org.assertj.core.api.Assertions.assertThat;
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

        monitor = new WorkerWellnessMonitor(redisTemplate, objectMapper);
        ReflectionTestUtils.setField(monitor, "heartbeatPattern", "worker:*:heartbeat");
    }

    @Test
    @DisplayName("Scans active worker heartbeats and marks recent pulses as healthy")
    void shouldIdentifyHealthyWorkers() throws Exception {
        HeartbeatPayload hb = new HeartbeatPayload("worker-go-01", "BUSY", "cpt_99", Instant.now().minusSeconds(5));
        String json = objectMapper.writeValueAsString(hb);

        when(redisTemplate.keys("worker:*:heartbeat")).thenReturn(Set.of("worker:worker-go-01:heartbeat"));
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

        when(redisTemplate.keys("worker:*:heartbeat")).thenReturn(Set.of("worker:worker-go-02:heartbeat"));
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
}
