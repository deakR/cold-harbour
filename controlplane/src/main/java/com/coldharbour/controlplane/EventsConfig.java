package com.coldharbour.controlplane;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.listener.ChannelTopic;
import org.springframework.data.redis.listener.RedisMessageListenerContainer;
import org.springframework.web.socket.config.annotation.EnableWebSocket;
import org.springframework.web.socket.config.annotation.WebSocketConfigurer;
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry;

@Configuration
@EnableWebSocket
public class EventsConfig implements WebSocketConfigurer {

	static final String EVENTS_CHANNEL = "coldharbour:events";

	private final EventsSocketHandler socketHandler;
	private final EventsHandshake handshake;

	public EventsConfig(EventsSocketHandler socketHandler, EventsHandshake handshake) {
		this.socketHandler = socketHandler;
		this.handshake = handshake;
	}

	@Override
	public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
		registry.addHandler(socketHandler, "/ws/events")
				.addInterceptors(handshake);
	}

	@Bean
	public RedisMessageListenerContainer eventsListenerContainer(
			RedisConnectionFactory connectionFactory,
			EventsRelay relay) {
		RedisMessageListenerContainer container = new RedisMessageListenerContainer();
		container.setConnectionFactory(connectionFactory);
		container.addMessageListener(relay, new ChannelTopic(EVENTS_CHANNEL));
		return container;
	}
}
