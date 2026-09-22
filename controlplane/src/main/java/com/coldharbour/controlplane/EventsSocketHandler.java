package com.coldharbour.controlplane;

import java.util.UUID;

import org.springframework.stereotype.Component;
import org.springframework.web.socket.CloseStatus;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.handler.TextWebSocketHandler;

@Component
public final class EventsSocketHandler extends TextWebSocketHandler {

	private final EventsHub hub;

	public EventsSocketHandler(EventsHub hub) {
		this.hub = hub;
	}

	@Override
	public void afterConnectionEstablished(WebSocketSession session) {
		UUID tenantId = (UUID) session.getAttributes().get(ApiKeyFilter.ATTR_TENANT);
		if (tenantId == null) {
			try {
				session.close(CloseStatus.NOT_ACCEPTABLE);
			} catch (Exception ignored) {
			}
			return;
		}
		hub.register(tenantId, session);
	}

	@Override
	public void afterConnectionClosed(WebSocketSession session, CloseStatus status) {
		hub.unregister(session);
	}
}
