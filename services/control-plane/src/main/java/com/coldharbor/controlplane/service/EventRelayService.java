package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.model.EventMessage;
import com.coldharbor.controlplane.websocket.EventWebSocketHandler;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.connection.Message;
import org.springframework.data.redis.connection.MessageListener;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.messaging.simp.SimpMessagingTemplate;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.UUID;

/**
 * Event relay subscribing to Redis Pub/Sub channel coldharbor:events,
 * updating compartment metadata, recording durable audit records, and
 * streaming events over WebSocket to clients.
 */
@Service
public class EventRelayService implements MessageListener {

    private static final Logger log = LoggerFactory.getLogger(EventRelayService.class);

    private final StringRedisTemplate redisTemplate;
    private final AuditService auditService;
    private final EventWebSocketHandler webSocketHandler;
    private final ObjectMapper objectMapper;

    @Autowired(required = false)
    private SimpMessagingTemplate messagingTemplate;

    @Value("${coldharbor.redis.meta-prefix:compartment:}")
    private String metaPrefix;

    @Value("${coldharbor.redis.archive-prefix:archive:}")
    private String archivePrefix;

    public EventRelayService(StringRedisTemplate redisTemplate,
                             AuditService auditService,
                             EventWebSocketHandler webSocketHandler,
                             ObjectMapper objectMapper) {
        this.redisTemplate = redisTemplate;
        this.auditService = auditService;
        this.webSocketHandler = webSocketHandler;
        this.objectMapper = objectMapper;
    }

    @Override
    public void onMessage(Message message, byte[] pattern) {
        try {
            String jsonPayload = new String(message.getBody(), StandardCharsets.UTF_8);
            processEvent(jsonPayload);
        } catch (Exception e) {
            log.error("Error processing Redis event: {}", e.getMessage(), e);
        }
    }

    public void processEvent(String jsonPayload) {
        try {
            EventMessage event = objectMapper.readValue(jsonPayload, EventMessage.class);
            String compartmentId = event.getCompartmentId();
            String toState = event.getEffectiveToState();

            log.info("Received event for compartment {}: {} -> {} (progress: {}%)",
                    compartmentId, event.getEffectiveFromState(), toState, event.getEffectiveProgress());

            // 1. Update live compartment metadata in Redis
            updateCompartmentMeta(event);

            // 2. Auto-record completed/failed jobs to PostgreSQL audit_records
            if ("PURGED".equalsIgnoreCase(toState) || "ARCHIVED".equalsIgnoreCase(toState)
                    || "COMPLETED".equalsIgnoreCase(toState) || "FAILED".equalsIgnoreCase(toState)) {
                recordDurableAudit(event);
            }

            // 3. Broadcast to native WebSocket clients
            webSocketHandler.broadcast(jsonPayload);

            // 4. Also broadcast to STOMP clients on /topic/events if active
            if (messagingTemplate != null) {
                try {
                    messagingTemplate.convertAndSend("/topic/events", event);
                } catch (Exception e) {
                    log.debug("STOMP relay skipped or unavailable: {}", e.getMessage());
                }
            }

        } catch (Exception e) {
            log.error("Failed to parse or relay event message: {}", e.getMessage(), e);
        }
    }

    private void updateCompartmentMeta(EventMessage event) {
        if (event.getCompartmentId() == null) {
            return;
        }
        try {
            String key = metaPrefix + event.getCompartmentId() + ":meta";
            String existingJson = redisTemplate.opsForValue().get(key);
            CompartmentDetail detail;
            if (existingJson != null) {
                detail = objectMapper.readValue(existingJson, CompartmentDetail.class);
            } else {
                detail = new CompartmentDetail();
                detail.setCompartmentId(event.getCompartmentId());
                detail.setCreatedAt(Instant.now());
                detail.setContext("INNIE");
                detail.setOwnerId("system");
                detail.setTaskType("TASK");
            }

            detail.setState(event.getEffectiveToState());
            detail.setProgress(event.getEffectiveProgress());
            detail.setUpdatedAt(event.getTimestamp() != null ? event.getTimestamp() : Instant.now());
            if (event.getDetails() != null) {
                detail.setDetails(event.getDetails());
            }

            redisTemplate.opsForValue().set(key, objectMapper.writeValueAsString(detail), Duration.ofHours(24));
        } catch (Exception e) {
            log.warn("Failed to update compartment meta for {}: {}", event.getCompartmentId(), e.getMessage());
        }
    }

    private void recordDurableAudit(EventMessage event) {
        String compartmentId = event.getCompartmentId();
        String state = event.getEffectiveToState();
        // Only terminal success states persist; intermediate events (RUNNING,
        // CHECKPOINT, COMPLETED) would otherwise race into duplicate rows.
        if (!"ARCHIVED".equalsIgnoreCase(state) && !"PURGED".equalsIgnoreCase(state)
                && !"FAILED".equalsIgnoreCase(state)) {
            return;
        }
        try {
            String archiveKey = archivePrefix + compartmentId;
            String archiveJson = redisTemplate.opsForValue().get(archiveKey);

            if (archiveJson != null && !archiveJson.trim().isEmpty()) {
                DeadDropPayload deadDrop = objectMapper.readValue(archiveJson, DeadDropPayload.class);
                auditService.recordCompletedJob(compartmentId, deadDrop, event.getDetails());
            } else if ("FAILED".equalsIgnoreCase(event.getEffectiveToState())) {
                AuditRecord failedRecord = new AuditRecord(
                        UUID.randomUUID(),
                        compartmentId,
                        "unknown",
                        "INNIE",
                        "TASK",
                        "FAILED",
                        "",
                        0L,
                        Instant.now(),
                        Instant.now(),
                        "{\"error\":\"Task failed\",\"details\":\"" + (event.getDetails() != null ? event.getDetails() : "") + "\"}"
                );
                auditService.saveAuditRecord(failedRecord);
            }
        } catch (Exception e) {
            log.warn("Failed to record durable audit for compartment {}: {}", compartmentId, e.getMessage());
        }
    }
}
