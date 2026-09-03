package com.coldharbor.controlplane.security;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.annotation.Order;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import java.io.IOException;
import java.util.Map;

/**
 * Filter enforcing X-Context-Clearance header on protected API endpoints.
 */
@Component
@Order(1)
public class ContextClearanceFilter extends OncePerRequestFilter {

    public static final String CLEARANCE_HEADER = "X-Context-Clearance";
    public static final String FALLBACK_HEADER_1 = "X-Clearance";
    public static final String FALLBACK_HEADER_2 = "X-ColdHarbor-Context";
    public static final String CLEARANCE_ATTRIBUTE = "coldharbor_clearance";

    private final ObjectMapper objectMapper = new ObjectMapper();

    @Override
    protected boolean shouldNotFilter(HttpServletRequest request) {
        String path = request.getRequestURI();
        // Allow WebSocket handshake, error dispatches, static assets, and actuator
        return path.startsWith("/ws")
                || path.startsWith("/actuator")
                || path.startsWith("/error")
                || path.equals("/")
                || path.equals("/favicon.ico");
    }

    @Override
    protected void doFilterInternal(HttpServletRequest request,
                                    HttpServletResponse response,
                                    FilterChain filterChain) throws ServletException, IOException {
        String headerValue = request.getHeader(CLEARANCE_HEADER);
        if (headerValue == null || headerValue.trim().isEmpty()) {
            headerValue = request.getHeader(FALLBACK_HEADER_1);
        }
        if (headerValue == null || headerValue.trim().isEmpty()) {
            headerValue = request.getHeader(FALLBACK_HEADER_2);
        }

        ContextClearance clearance = ContextClearance.fromString(headerValue);
        if (clearance == null) {
            sendForbiddenResponse(response, "Missing or invalid " + CLEARANCE_HEADER + " header. Allowed values: INNIE, OUTIE, SYSTEM, ADMIN.");
            return;
        }

        ClearanceContext.setClearance(clearance);
        request.setAttribute(CLEARANCE_ATTRIBUTE, clearance);
        try {
            filterChain.doFilter(request, response);
        } finally {
            ClearanceContext.clear();
        }
    }

    private void sendForbiddenResponse(HttpServletResponse response, String message) throws IOException {
        response.setStatus(HttpStatus.FORBIDDEN.value());
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        Map<String, Object> body = Map.of(
                "status", HttpStatus.FORBIDDEN.value(),
                "error", "Forbidden",
                "message", message
        );
        response.getWriter().write(objectMapper.writeValueAsString(body));
    }
}
