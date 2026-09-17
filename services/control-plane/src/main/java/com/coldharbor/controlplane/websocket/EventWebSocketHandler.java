package com.coldharbor.controlplane.websocket;

import com.coldharbor.controlplane.security.ContextClearance;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.SubProtocolCapable;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import java.io.IOException;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Native WebSocket handler streaming real-time event updates to connected clients.
 */
@Component
public class EventWebSocketHandler extends TextWebSocketHandler implements SubProtocolCapable {

    private static final Logger log = LoggerFactory.getLogger(EventWebSocketHandler.class);

    private final Set<WebSocketSession> sessions = ConcurrentHashMap.newKeySet();

    @Override
    public List<String> getSubProtocols() {
        return List.of("coldharbor");
    }

    @Override
    public void afterConnectionEstablished(WebSocketSession session) {
        sessions.add(session);
        log.info("WebSocket client connected: session id {}", session.getId());
    }

    @Override
    public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
        sessions.remove(session);
        log.info("WebSocket client disconnected: session id {}, status {}", session.getId(), status);
    }

    @Override
    public void handleTransportError(WebSocketSession session, Throwable exception) {
        log.warn("WebSocket transport error for session {}: {}", session.getId(), exception.getMessage());
        sessions.remove(session);
        try {
            if (session.isOpen()) {
                session.close(CloseStatus.SERVER_ERROR);
            }
        } catch (IOException ignored) {
        }
    }

    @Override
    protected void handleTextMessage(WebSocketSession session, TextMessage message) throws Exception {
        // Echo or handle client ping
        if ("ping".equalsIgnoreCase(message.getPayload().trim())) {
            session.sendMessage(new TextMessage("pong"));
        }
    }

    /**
     * Broadcasts an event only to sessions whose authenticated clearance can
     * access the event's compartment context.
     */
    public void broadcast(String jsonPayload, String eventContext) {
        if (sessions.isEmpty() || jsonPayload == null || eventContext == null) {
            return;
        }

        TextMessage textMessage = new TextMessage(jsonPayload);
        for (WebSocketSession session : sessions) {
            if (!session.isOpen()) {
                sessions.remove(session);
                continue;
            }
            Object sessionClearance = session.getAttributes()
                    .get(ClearanceHandshakeInterceptor.CLEARANCE_SESSION_ATTRIBUTE);
            ContextClearance clearance = sessionClearance instanceof ContextClearance
                    ? (ContextClearance) sessionClearance
                    : ContextClearance.fromString(String.valueOf(sessionClearance));
            if (clearance != null && clearance.canAccess(eventContext)) {
                try {
                    synchronized (session) {
                        session.sendMessage(textMessage);
                    }
                } catch (Exception e) {
                    log.warn("Failed to send WebSocket message to session {}: {}", session.getId(), e.getMessage());
                }
            }
        }
    }

    /**
     * Retained for source compatibility. Context-free broadcasts are deliberately
     * dropped because they cannot be authorized safely.
     */
    @Deprecated
    public void broadcast(String jsonPayload) {
        log.warn("Dropped context-free WebSocket broadcast");
    }

    public int getConnectedClientsCount() {
        return sessions.size();
    }
}
