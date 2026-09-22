package com.coldharbour.controlplane;

import java.io.IOException;
import java.util.Map;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArraySet;

import org.springframework.stereotype.Component;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;

@Component
public final class EventsHub {

	private final Map<UUID, Set<WebSocketSession>> sessions = new ConcurrentHashMap<>();

	public void register(UUID tenantId, WebSocketSession session) {
		sessions.computeIfAbsent(tenantId, id -> new CopyOnWriteArraySet<>()).add(session);
	}

	public void unregister(WebSocketSession session) {
		UUID tenantId = (UUID) session.getAttributes().get(ApiKeyFilter.ATTR_TENANT);
		if (tenantId == null) {
			return;
		}
		sessions.computeIfPresent(tenantId, (id, set) -> {
			set.remove(session);
			return set.isEmpty() ? null : set;
		});
	}

	public void deliver(UUID tenantId, String payload) {
		Set<WebSocketSession> set = sessions.get(tenantId);
		if (set == null || set.isEmpty()) {
			return;
		}
		TextMessage message = new TextMessage(payload);
		for (WebSocketSession session : set) {
			if (!session.isOpen()) {
				continue;
			}
			try {
				session.sendMessage(message);
			} catch (IOException ignored) {
			}
		}
	}
}
