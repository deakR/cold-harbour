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

    @Test
    @DisplayName("Redrive moves DLQ entries back to the main stream and clears them")
    @SuppressWarnings({"unchecked", "rawtypes"})
    void shouldRedriveDlqEntries() throws Exception {
        org.springframework.data.redis.core.StreamOperations streamOps =
                mock(org.springframework.data.redis.core.StreamOperations.class);
        when(redisTemplate.opsForStream()).thenReturn(streamOps);
        ReflectionTestUtils.setField(monitor, "dlqStreamName", "coldharbor:jobs:dlq");
        ReflectionTestUtils.setField(monitor, "streamName", "coldharbor:jobs");

        String jobData = "{\"compartmentId\":\"cpt_dlq_1\",\"context\":\"INNIE\","
                + "\"ownerId\":\"usr_1\",\"taskType\":\"DATA_REDUCTION\","
                + "\"payload\":{\"batchSize\":100},\"maxRetries\":3,\"timeoutSeconds\":300,"
                + "\"createdAt\":\"2026-09-03T10:00:00Z\"}";
        org.springframework.data.redis.connection.stream.MapRecord<String, String, String> record =
                mock(org.springframework.data.redis.connection.stream.MapRecord.class);
        org.springframework.data.redis.connection.stream.RecordId recordId =
                mock(org.springframework.data.redis.connection.stream.RecordId.class);
        when(record.getId()).thenReturn(recordId);
        java.util.Map<String, String> fields = new java.util.LinkedHashMap<>();
        fields.put("compartmentId", "cpt_dlq_1");
        fields.put("jobData", jobData);
        when(record.getValue()).thenReturn((java.util.Map) fields);
        when(streamOps.range(org.mockito.ArgumentMatchers.anyString(),
                org.mockito.ArgumentMatchers.any(),
                org.mockito.ArgumentMatchers.any())).thenReturn(java.util.List.of(record));

        java.util.Map<String, Object> result = monitor.redriveDlq(10);

        assertThat(result.get("redriven")).isEqualTo(1);
        verify(streamOps, times(1)).add(any());
        verify(streamOps, times(1)).delete(eq("coldharbor:jobs:dlq"),
                any(org.springframework.data.redis.connection.stream.RecordId[].class));
    }
}
