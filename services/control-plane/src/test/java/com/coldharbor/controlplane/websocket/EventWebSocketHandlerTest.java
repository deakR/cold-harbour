package com.coldharbor.controlplane.websocket;

import com.coldharbor.controlplane.security.ContextClearance;
import org.junit.jupiter.api.Test;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

import java.util.HashMap;
import java.util.Map;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

class EventWebSocketHandlerTest {

    @Test
    void broadcastsOnlyToSessionsAuthorizedForEventContext() throws Exception {
        EventWebSocketHandler handler = new EventWebSocketHandler();
        WebSocketSession innie = session("innie", ContextClearance.INNIE);
        WebSocketSession outie = session("outie", ContextClearance.OUTIE);
        WebSocketSession admin = session("admin", ContextClearance.ADMIN);
        handler.afterConnectionEstablished(innie);
        handler.afterConnectionEstablished(outie);
        handler.afterConnectionEstablished(admin);

        handler.broadcast("{\"eventId\":\"evt_1\"}", "INNIE");

        verify(innie).sendMessage(any(TextMessage.class));
        verify(admin).sendMessage(any(TextMessage.class));
        verify(outie, never()).sendMessage(any());

        handler.broadcast("{\"eventId\":\"evt_2\"}", "OUTIE");

        verify(outie).sendMessage(any(TextMessage.class));
        verify(admin, times(2)).sendMessage(any(TextMessage.class));
    }

    @Test
    void dropsContextFreeCompatibilityBroadcast() throws Exception {
        EventWebSocketHandler handler = new EventWebSocketHandler();
        WebSocketSession session = session("admin", ContextClearance.ADMIN);
        handler.afterConnectionEstablished(session);

        handler.broadcast("{}");

        verify(session, never()).sendMessage(any());
    }

    private static WebSocketSession session(String id, ContextClearance clearance) {
        WebSocketSession session = mock(WebSocketSession.class);
        Map<String, Object> attributes = new HashMap<>();
        attributes.put(ClearanceHandshakeInterceptor.CLEARANCE_SESSION_ATTRIBUTE, clearance);
        when(session.getId()).thenReturn(id);
        when(session.getAttributes()).thenReturn(attributes);
        when(session.isOpen()).thenReturn(true);
        return session;
    }
}
