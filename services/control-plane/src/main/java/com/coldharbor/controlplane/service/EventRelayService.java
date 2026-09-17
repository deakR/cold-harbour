package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.model.DeadDropPayload;
import com.coldharbor.controlplane.model.EventMessage;
import com.coldharbor.controlplane.websocket.EventWebSocketHandler;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.connection.Message;
import org.springframework.data.redis.connection.MessageListener;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
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

            // Resolve security metadata before mutating state or publishing.
            CompartmentDetail securityMetadata = enrichEventContext(event);

            // 1. Update live compartment metadata in Redis
            updateCompartmentMeta(event);

            // 2. Auto-record completed/failed jobs to PostgreSQL audit_records
            if ("PURGED".equalsIgnoreCase(toState)) {
                recordDurableAudit(event, securityMetadata);
            }

            // 3. Broadcast only when the event context is known and authorizable.
            if (event.getContext() != null) {
                webSocketHandler.broadcast(objectMapper.writeValueAsString(event), event.getContext());
            } else {
                log.warn("Dropped event broadcast for compartment {} because security context is unavailable",
                        compartmentId);
            }

        } catch (Exception e) {
            log.error("Failed to parse or relay event message: {}", e.getMessage(), e);
        }
    }

    /**
     * Replays a durable event into the query projection without duplicating
     * live WebSocket fanout or terminal audit writes.
     */
    public void processDurableEvent(String jsonPayload) throws Exception {
        EventMessage event = objectMapper.readValue(jsonPayload, EventMessage.class);
        enrichEventContext(event);
        updateCompartmentMeta(event);
    }

    private CompartmentDetail enrichEventContext(EventMessage event) {
        CompartmentDetail detail = null;
        try {
            String metaJson = redisTemplate.opsForValue()
                    .get(metaPrefix + event.getCompartmentId() + ":meta");
            if (metaJson != null && !metaJson.trim().isEmpty()) {
                detail = objectMapper.readValue(metaJson, CompartmentDetail.class);
            }
            if (detail == null) {
                String archiveJson = redisTemplate.opsForValue()
                        .get(archivePrefix + event.getCompartmentId());
                if (archiveJson != null && !archiveJson.trim().isEmpty()) {
                    DeadDropPayload deadDrop = objectMapper.readValue(archiveJson, DeadDropPayload.class);
                    detail = new CompartmentDetail();
                    detail.setCompartmentId(event.getCompartmentId());
                    detail.setContext(deadDrop.getContext());
                    detail.setOwnerId(deadDrop.getOwnerId());
                    detail.setTaskType(deadDrop.getTaskType());
                }
            }
        } catch (Exception e) {
            log.warn("Failed to resolve event security metadata for {}: {}",
                    event.getCompartmentId(), e.getMessage());
        }
        if (detail != null) {
            event.setContext(detail.getContext());
            event.setOwnerId(detail.getOwnerId());
            event.setTaskType(detail.getTaskType());
        }
        return detail;
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
                if (detail.getUpdatedAt() != null && event.getTimestamp() != null
                        && event.getTimestamp().isBefore(detail.getUpdatedAt())) {
                    log.debug("Ignored stale event {} for compartment {}",
                            event.getEventId(), event.getCompartmentId());
                    return;
                }
            } else {
                if (event.getContext() == null || event.getOwnerId() == null || event.getTaskType() == null) {
                    log.warn("Not creating metadata for {} without trusted context/owner/task",
                            event.getCompartmentId());
                    return;
                }
                detail = new CompartmentDetail();
                detail.setCompartmentId(event.getCompartmentId());
                detail.setCreatedAt(Instant.now());
                detail.setContext(event.getContext());
                detail.setOwnerId(event.getOwnerId());
                detail.setTaskType(event.getTaskType());
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

    private void recordDurableAudit(EventMessage event, CompartmentDetail metadata) {
        String compartmentId = event.getCompartmentId();
        String state = event.getEffectiveToState();
        if (!"PURGED".equalsIgnoreCase(state) && !"FAILED".equalsIgnoreCase(state)) {
            return;
        }
        try {
            String archiveKey = archivePrefix + compartmentId;
            String archiveJson = redisTemplate.opsForValue().get(archiveKey);

            if (archiveJson != null && !archiveJson.trim().isEmpty()) {
                DeadDropPayload deadDrop = objectMapper.readValue(archiveJson, DeadDropPayload.class);
                auditService.recordCompletedJob(compartmentId, deadDrop, event.getDetails(), state.toUpperCase());
            } else if ("FAILED".equalsIgnoreCase(event.getEffectiveToState())) {
                if (metadata == null || metadata.getContext() == null
                        || metadata.getOwnerId() == null || metadata.getTaskType() == null) {
                    log.warn("Skipped FAILED audit for {} because trusted security metadata is unavailable",
                            compartmentId);
                    return;
                }
                Map<String, Object> failureMetadata = new LinkedHashMap<>();
                failureMetadata.put("error", "Task failed");
                failureMetadata.put("details", event.getDetails() != null ? event.getDetails() : "");
                AuditRecord failedRecord = new AuditRecord(
                        UUID.randomUUID(),
                        compartmentId,
                        metadata.getOwnerId(),
                        metadata.getContext(),
                        metadata.getTaskType(),
                        "FAILED",
                        "",
                        0L,
                        Instant.now(),
                        Instant.now(),
                        objectMapper.writeValueAsString(failureMetadata)
                );
                auditService.saveAuditRecord(failedRecord);
            }
        } catch (Exception e) {
            log.warn("Failed to record durable audit for compartment {}: {}", compartmentId, e.getMessage());
        }
    }
}
