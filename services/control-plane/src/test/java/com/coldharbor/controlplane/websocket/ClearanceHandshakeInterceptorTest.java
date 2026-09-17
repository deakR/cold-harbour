package com.coldharbor.controlplane.websocket;

import com.coldharbor.controlplane.security.ApiKeyClearanceProperties;
import com.coldharbor.controlplane.security.ContextClearance;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpHeaders;
import org.springframework.http.HttpStatus;
import org.springframework.http.server.ServerHttpRequest;
import org.springframework.http.server.ServerHttpResponse;
import org.springframework.web.socket.WebSocketHandler;

import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

class ClearanceHandshakeInterceptorTest {

    @Test
    void authenticatesApiKeyAndStoresClearance() {
        ApiKeyClearanceProperties properties = new ApiKeyClearanceProperties();
        properties.setApiKeys(Map.of("secret-key", "OUTIE"));
        properties.setAllowUnsafeHeader(false);
        ClearanceHandshakeInterceptor interceptor = new ClearanceHandshakeInterceptor(properties);
        HttpHeaders headers = new HttpHeaders();
        headers.set("X-API-Key", "secret-key");
        ServerHttpRequest request = request(headers, "ws://localhost/ws/events");
        Map<String, Object> attributes = new HashMap<>();

        boolean accepted = interceptor.beforeHandshake(request, mock(ServerHttpResponse.class),
                mock(WebSocketHandler.class), attributes);

        assertThat(accepted).isTrue();
        assertThat(attributes.get(ClearanceHandshakeInterceptor.CLEARANCE_SESSION_ATTRIBUTE))
                .isEqualTo(ContextClearance.OUTIE);
    }

    @Test
    void authenticatesBrowserApiKeyFromWebSocketSubprotocol() {
        ApiKeyClearanceProperties properties = new ApiKeyClearanceProperties();
        properties.setApiKeys(Map.of("browser-secret", "INNIE"));
        properties.setAllowUnsafeHeader(false);
        ClearanceHandshakeInterceptor interceptor = new ClearanceHandshakeInterceptor(properties);
        String encoded = Base64.getUrlEncoder().withoutPadding()
                .encodeToString("browser-secret".getBytes(StandardCharsets.UTF_8));
        HttpHeaders headers = new HttpHeaders();
        headers.set("Sec-WebSocket-Protocol",
                "coldharbor, clearance.OUTIE, api-key." + encoded);
        Map<String, Object> attributes = new HashMap<>();

        boolean accepted = interceptor.beforeHandshake(
                request(headers, "ws://localhost/ws/events"),
                mock(ServerHttpResponse.class), mock(WebSocketHandler.class), attributes);

        assertThat(accepted).isTrue();
        assertThat(attributes.get(ClearanceHandshakeInterceptor.CLEARANCE_SESSION_ATTRIBUTE))
                .isEqualTo(ContextClearance.INNIE);
    }

    @Test
    void rejectsMissingApiKeyWhenUnsafeHeadersDisabled() {
        ApiKeyClearanceProperties properties = new ApiKeyClearanceProperties();
        properties.setApiKeys(Map.of("secret-key", "ADMIN"));
        properties.setAllowUnsafeHeader(false);
        ClearanceHandshakeInterceptor interceptor = new ClearanceHandshakeInterceptor(properties);
        ServerHttpResponse response = mock(ServerHttpResponse.class);

        boolean accepted = interceptor.beforeHandshake(
                request(new HttpHeaders(), "ws://localhost/ws/events"),
                response, mock(WebSocketHandler.class), new HashMap<>());

        assertThat(accepted).isFalse();
        verify(response).setStatusCode(HttpStatus.UNAUTHORIZED);
    }

    private static ServerHttpRequest request(HttpHeaders headers, String uri) {
        ServerHttpRequest request = mock(ServerHttpRequest.class);
        when(request.getHeaders()).thenReturn(headers);
        when(request.getURI()).thenReturn(URI.create(uri));
        return request;
    }
}
