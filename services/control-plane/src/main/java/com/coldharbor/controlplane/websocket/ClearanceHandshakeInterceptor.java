package com.coldharbor.controlplane.websocket;

import com.coldharbor.controlplane.security.ApiKeyClearanceProperties;
import com.coldharbor.controlplane.security.ContextClearance;
import com.coldharbor.controlplane.security.ContextClearanceFilter;
import org.springframework.http.HttpStatus;
import org.springframework.http.server.ServerHttpRequest;
import org.springframework.http.server.ServerHttpResponse;
import org.springframework.http.server.ServletServerHttpRequest;
import org.springframework.stereotype.Component;
import org.springframework.util.MultiValueMap;
import org.springframework.web.socket.WebSocketHandler;
import org.springframework.web.socket.server.HandshakeInterceptor;
import org.springframework.web.util.UriComponentsBuilder;

import java.util.Map;
import java.nio.charset.StandardCharsets;
import java.util.Base64;

/**
 * Applies the same API-key/clearance policy as the HTTP filter to WebSocket handshakes.
 */
@Component
public class ClearanceHandshakeInterceptor implements HandshakeInterceptor {

    public static final String CLEARANCE_SESSION_ATTRIBUTE = "coldharbor_clearance";

    private final ApiKeyClearanceProperties apiKeys;

    public ClearanceHandshakeInterceptor(ApiKeyClearanceProperties apiKeys) {
        this.apiKeys = apiKeys;
    }

    @Override
    public boolean beforeHandshake(ServerHttpRequest request, ServerHttpResponse response,
                                   WebSocketHandler wsHandler, Map<String, Object> attributes) {
        MultiValueMap<String, String> query =
                UriComponentsBuilder.fromUri(request.getURI()).build().getQueryParams();
        String protocolHeader = request.getHeaders().getFirst("Sec-WebSocket-Protocol");
        String apiKey = firstNonBlank(
                request.getHeaders().getFirst(ContextClearanceFilter.API_KEY_HEADER),
                protocolValue(protocolHeader, "api-key.", true));
        ContextClearance clearance;
        if (apiKey != null) {
            clearance = apiKeys.resolve(ApiKeyClearanceProperties.normalizeKey(apiKey));
            if (clearance == null) {
                response.setStatusCode(HttpStatus.UNAUTHORIZED);
                return false;
            }
        } else if (!apiKeys.isAllowUnsafeHeader()) {
            response.setStatusCode(HttpStatus.UNAUTHORIZED);
            return false;
        } else {
            String asserted = firstNonBlank(
                    request.getHeaders().getFirst(ContextClearanceFilter.CLEARANCE_HEADER),
                    request.getHeaders().getFirst(ContextClearanceFilter.FALLBACK_HEADER_1),
                    request.getHeaders().getFirst(ContextClearanceFilter.FALLBACK_HEADER_2),
                    protocolValue(protocolHeader, "clearance.", false),
                    query.getFirst("clearance"));
            clearance = ContextClearance.fromString(asserted);
        }

        if (clearance == null) {
            response.setStatusCode(HttpStatus.FORBIDDEN);
            return false;
        }
        attributes.put(CLEARANCE_SESSION_ATTRIBUTE, clearance);
        if (request instanceof ServletServerHttpRequest servletRequest) {
            Object traceId = servletRequest.getServletRequest()
                    .getAttribute(ContextClearanceFilter.TRACE_ATTRIBUTE);
            if (traceId != null) {
                attributes.put(ContextClearanceFilter.TRACE_ATTRIBUTE, traceId);
            }
        }
        return true;
    }

    @Override
    public void afterHandshake(ServerHttpRequest request, ServerHttpResponse response,
                               WebSocketHandler wsHandler, Exception exception) {
        // No post-handshake work.
    }

    private static String firstNonBlank(String... values) {
        for (String value : values) {
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }

    private static String protocolValue(String header, String prefix, boolean decode) {
        if (header == null) {
            return null;
        }
        for (String value : header.split(",")) {
            String token = value.trim();
            if (token.startsWith(prefix) && token.length() > prefix.length()) {
                String raw = token.substring(prefix.length());
                if (!decode) {
                    return raw;
                }
                try {
                    return new String(Base64.getUrlDecoder().decode(raw), StandardCharsets.UTF_8);
                } catch (IllegalArgumentException ignored) {
                    return null;
                }
            }
        }
        return null;
    }
}
