package com.coldharbour.controlplane;

import java.util.UUID;

import org.springframework.data.redis.connection.Message;
import org.springframework.data.redis.connection.MessageListener;
import org.springframework.stereotype.Component;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

@Component
public final class EventsRelay implements MessageListener {

	private final EventsHub hub;
	private final ObjectMapper mapper;

	public EventsRelay(EventsHub hub, ObjectMapper mapper) {
		this.hub = hub;
		this.mapper = mapper;
	}

	@Override
	public void onMessage(Message message, byte[] pattern) {
		String payload = new String(message.getBody());
		try {
			JsonNode node = mapper.readTree(payload);
			JsonNode tenantNode = node.get("tenantId");
			if (tenantNode == null || tenantNode.isNull() || tenantNode.asString().isEmpty()) {
				return;
			}
			UUID tenantId = UUID.fromString(tenantNode.asString());
			hub.deliver(tenantId, payload);
		} catch (Exception ignored) {
		}
	}
}
