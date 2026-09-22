package com.coldharbour.controlplane;

import java.util.Map;
import java.util.Optional;

import org.springframework.http.server.ServerHttpRequest;
import org.springframework.http.server.ServerHttpResponse;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.WebSocketHandler;
import org.springframework.web.socket.server.HandshakeInterceptor;

@Component
public final class EventsHandshake implements HandshakeInterceptor {

	private final ApiKeys apiKeys;

	public EventsHandshake(ApiKeys apiKeys) {
		this.apiKeys = apiKeys;
	}

	@Override
	public boolean beforeHandshake(ServerHttpRequest request, ServerHttpResponse response,
			WebSocketHandler wsHandler, Map<String, Object> attributes) {
		String raw = request.getHeaders().getFirst("X-API-Key");
		Optional<ApiKeys.Principal> principal = apiKeys.authenticate(raw);
		if (principal.isEmpty()) {
			return false;
		}
		attributes.put(ApiKeyFilter.ATTR_TENANT, principal.get().tenantId());
		return true;
	}

	@Override
	public void afterHandshake(ServerHttpRequest request, ServerHttpResponse response,
			WebSocketHandler wsHandler, Exception exception) {
	}
}
