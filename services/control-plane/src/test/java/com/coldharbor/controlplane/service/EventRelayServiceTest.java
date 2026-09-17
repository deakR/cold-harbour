package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.model.EventMessage;
import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.websocket.EventWebSocketHandler;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import org.springframework.data.redis.connection.DefaultMessage;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.test.util.ReflectionTestUtils;

import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Map;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class EventRelayServiceTest {

    private StringRedisTemplate redisTemplate;
    private ValueOperations<String, String> valueOperations;
    private AuditService auditService;
    private EventWebSocketHandler webSocketHandler;
    private ObjectMapper objectMapper;
    private EventRelayService eventRelayService;

    @BeforeEach
    void setUp() {
        redisTemplate = mock(StringRedisTemplate.class);
        valueOperations = mock(ValueOperations.class);
        when(redisTemplate.opsForValue()).thenReturn(valueOperations);

        auditService = mock(AuditService.class);
        webSocketHandler = mock(EventWebSocketHandler.class);
        objectMapper = new ObjectMapper();
        objectMapper.registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule());
        eventRelayService = new EventRelayService(redisTemplate, auditService, webSocketHandler, objectMapper);
        ReflectionTestUtils.setField(eventRelayService, "metaPrefix", "compartment:");
        ReflectionTestUtils.setField(eventRelayService, "archivePrefix", "archive:");
    }

    @Test
    @DisplayName("Processes event, updates Redis meta, and broadcasts to WebSocket")
    void shouldProcessEventAndBroadcast() throws Exception {
        EventMessage event = new EventMessage();
        event.setEventId("evt_1");
        event.setCompartmentId("cpt_10");
        event.setWorkerId("worker-go-01");
        event.setFromState("RUNNING");
        event.setToState("CHECKPOINT");
        event.setCheckpointPct(50);
        event.setTimestamp(Instant.now());
        event.setDetails("Checkpoint 50% reached");

        CompartmentDetail metadata = new CompartmentDetail();
        metadata.setCompartmentId("cpt_10");
        metadata.setContext("INNIE");
        metadata.setOwnerId("usr_1");
        metadata.setTaskType("DATA_REDUCTION");
        when(valueOperations.get("compartment:cpt_10:meta"))
                .thenReturn(objectMapper.writeValueAsString(metadata));

        String json = objectMapper.writeValueAsString(event);
        DefaultMessage message = new DefaultMessage("coldharbor:events".getBytes(StandardCharsets.UTF_8), json.getBytes(StandardCharsets.UTF_8));

        eventRelayService.onMessage(message, null);

        // Verify WebSocket broadcast
        verify(webSocketHandler, times(1)).broadcast(anyString(), eq("INNIE"));

        // Verify meta updated in Redis
        verify(valueOperations, times(1)).set(eq("compartment:cpt_10:meta"), anyString(), any());
    }

    @Test
    @DisplayName("PURGED event triggers durable audit record persistence from Dead Drop")
    void shouldTriggerAuditOnPurgedEvent() throws Exception {
        EventMessage event = new EventMessage();
        event.setEventId("evt_2");
        event.setCompartmentId("cpt_purged");
        event.setFromState("ARCHIVED");
        event.setToState("PURGED");
        event.setTimestamp(Instant.now());
        event.setDetails("Scratchpad purged atomically");

        DeadDropPayload deadDrop = new DeadDropPayload();
        deadDrop.setCompartmentId("cpt_purged");
        deadDrop.setOwnerId("usr_1");
        deadDrop.setContext("INNIE");
        deadDrop.setChecksum("sha256_dead_drop");
        deadDrop.setOutput(Map.of("res", 1));

        String deadDropJson = objectMapper.writeValueAsString(deadDrop);
        when(valueOperations.get("archive:cpt_purged")).thenReturn(deadDropJson);

        String eventJson = objectMapper.writeValueAsString(event);
        DefaultMessage message = new DefaultMessage("coldharbor:events".getBytes(StandardCharsets.UTF_8), eventJson.getBytes(StandardCharsets.UTF_8));

        eventRelayService.onMessage(message, null);

        verify(auditService, times(1)).recordCompletedJob(eq("cpt_purged"),
                any(DeadDropPayload.class), eq("Scratchpad purged atomically"), eq("PURGED"));
    }

    @Test
    void shouldDropBroadcastWhenContextCannotBeResolved() throws Exception {
        EventMessage event = new EventMessage();
        event.setCompartmentId("cpt_unknown");
        event.setToState("RUNNING");

        eventRelayService.processEvent(objectMapper.writeValueAsString(event));

        verify(webSocketHandler, never()).broadcast(anyString(), anyString());
        verify(auditService, never()).saveAuditRecord(any());
    }

    @Test
    void shouldDeferFailedAuditToDurableAuditStream() throws Exception {
        CompartmentDetail metadata = new CompartmentDetail();
        metadata.setCompartmentId("cpt_failed");
        metadata.setContext("OUTIE");
        metadata.setOwnerId("usr_failure");
        metadata.setTaskType("REPORT");
        when(valueOperations.get("compartment:cpt_failed:meta"))
                .thenReturn(objectMapper.writeValueAsString(metadata));
        EventMessage event = new EventMessage();
        event.setCompartmentId("cpt_failed");
        event.setToState("FAILED");
        event.setDetails("quote: \"safe\"");

        eventRelayService.processEvent(objectMapper.writeValueAsString(event));

        verify(auditService, never()).saveAuditRecord(any());
        verify(auditService, never()).recordCompletedJob(anyString(), any(), anyString(), anyString());
    }

    @Test
    void shouldNotPersistArchivedEventAsPurged() throws Exception {
        CompartmentDetail metadata = new CompartmentDetail();
        metadata.setCompartmentId("cpt_archived");
        metadata.setContext("INNIE");
        metadata.setOwnerId("usr_1");
        metadata.setTaskType("TASK");
        when(valueOperations.get("compartment:cpt_archived:meta"))
                .thenReturn(objectMapper.writeValueAsString(metadata));
        EventMessage event = new EventMessage();
        event.setCompartmentId("cpt_archived");
        event.setToState("ARCHIVED");

        eventRelayService.processEvent(objectMapper.writeValueAsString(event));

        verify(auditService, never()).recordCompletedJob(anyString(), any(), anyString(), anyString());
    }
}
