package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.CreateCompartmentRequest;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.RecordId;
import org.springframework.data.redis.core.StreamOperations;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.test.util.ReflectionTestUtils;

import java.time.Duration;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class JobDispatchServiceTest {

    private StringRedisTemplate redisTemplate;
    private StreamOperations<String, Object, Object> streamOperations;
    private ValueOperations<String, String> valueOperations;
    private ObjectMapper objectMapper;
    private JobDispatchService dispatchService;

    @BeforeEach
    void setUp() {
        redisTemplate = mock(StringRedisTemplate.class);
        streamOperations = mock(StreamOperations.class);
        valueOperations = mock(ValueOperations.class);

        when(redisTemplate.opsForStream()).thenReturn(streamOperations);
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

        when(streamOperations.add(any(MapRecord.class))).thenReturn(RecordId.of("1720000000-0"));

        CompartmentDetail result = dispatchService.dispatchJob(req);

        assertThat(result).isNotNull();
        assertThat(result.getCompartmentId()).isEqualTo("cpt_abc123");
        assertThat(result.getState()).isEqualTo("QUEUED");
        assertThat(result.getProgress()).isEqualTo(0);

        // Verify stream write
        ArgumentCaptor<MapRecord> captor = ArgumentCaptor.forClass(MapRecord.class);
        verify(streamOperations, times(1)).add(captor.capture());

        MapRecord record = captor.getValue();
        assertThat(record.getStream()).isEqualTo("coldharbor:jobs");
        Map<String, String> value = (Map<String, String>) record.getValue();
        assertThat(value.get("compartmentId")).isEqualTo("cpt_abc123");
        assertThat(value.get("context")).isEqualTo("INNIE");
        assertThat(value.get("taskType")).isEqualTo("DATA_REDUCTION");
        assertThat(value.get("ownerId")).isEqualTo("usr_9918");

        // Verify metadata set in Redis
        verify(valueOperations, times(1)).set(
                eq("compartment:cpt_abc123:meta"),
                anyString(),
                any(Duration.class)
        );
    }
}
