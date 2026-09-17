package com.coldharbor.controlplane.config;

import com.coldharbor.controlplane.websocket.EventWebSocketHandler;
import com.coldharbor.controlplane.websocket.ClearanceHandshakeInterceptor;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.socket.config.annotation.EnableWebSocket;
import org.springframework.web.socket.config.annotation.WebSocketConfigurer;
import org.springframework.web.socket.config.annotation.WebSocketHandlerRegistry;

/**
 * Native WebSocket configuration registering /ws/events and /ws endpoints.
 */
@Configuration
@EnableWebSocket
public class WebSocketConfig implements WebSocketConfigurer {

    private final EventWebSocketHandler eventWebSocketHandler;
    private final ClearanceHandshakeInterceptor clearanceHandshakeInterceptor;

    @Value("${coldharbor.websocket.allowed-origins:http://localhost:3000,http://localhost:5173,http://127.0.0.1:3000,http://127.0.0.1:5173}")
    private String[] allowedOrigins;

    public WebSocketConfig(EventWebSocketHandler eventWebSocketHandler,
                           ClearanceHandshakeInterceptor clearanceHandshakeInterceptor) {
        this.eventWebSocketHandler = eventWebSocketHandler;
        this.clearanceHandshakeInterceptor = clearanceHandshakeInterceptor;
    }

    @Override
    public void registerWebSocketHandlers(WebSocketHandlerRegistry registry) {
        registry.addHandler(eventWebSocketHandler, "/ws/events", "/ws")
                .addInterceptors(clearanceHandshakeInterceptor)
                .setAllowedOrigins(allowedOrigins);
    }
}
