package com.coldharbour.controlplane;

import java.util.UUID;

import org.springframework.data.redis.connection.Message;
import org.springframework.data.redis.connection.MessageListener;
import org.springframework.stereotype.Component;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

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
			if (tenantNode == null || tenantNode.isNull() || tenantNode.asText().isEmpty()) {
				return;
			}
			UUID tenantId = UUID.fromString(tenantNode.asText());
			hub.deliver(tenantId, payload);
		} catch (Exception ignored) {
		}
	}
}
