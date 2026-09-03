package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.model.EventMessage;
import com.coldharbor.controlplane.websocket.EventWebSocketHandler;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
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

        String json = objectMapper.writeValueAsString(event);
        DefaultMessage message = new DefaultMessage("coldharbor:events".getBytes(StandardCharsets.UTF_8), json.getBytes(StandardCharsets.UTF_8));

        eventRelayService.onMessage(message, null);

        // Verify WebSocket broadcast
        verify(webSocketHandler, times(1)).broadcast(json);

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

        verify(auditService, times(1)).recordCompletedJob(eq("cpt_purged"), any(DeadDropPayload.class), eq("Scratchpad purged atomically"));
    }
}
