package com.coldharbor.controlplane.security;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.springframework.core.annotation.Order;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.HttpStatus;
import org.springframework.http.MediaType;
import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;
import org.slf4j.MDC;

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
    public static final String API_KEY_HEADER = "X-API-Key";
    public static final String TRACE_HEADER = "X-Trace-Id";
    public static final String CLEARANCE_ATTRIBUTE = "coldharbor_clearance";
    public static final String TRACE_ATTRIBUTE = "coldharbor_trace_id";

    private final ObjectMapper objectMapper = new ObjectMapper();
    private final ApiKeyClearanceProperties apiKeys;
    /** Fixed-window dispatch counters per client IP: [windowStartMillis, count]. */
    private final java.util.concurrent.ConcurrentHashMap<String, long[]> dispatchWindows =
            new java.util.concurrent.ConcurrentHashMap<>();

    public ContextClearanceFilter() {
        this(new ApiKeyClearanceProperties());
    }

    @Autowired
    public ContextClearanceFilter(ApiKeyClearanceProperties apiKeys) {
        this.apiKeys = apiKeys != null ? apiKeys : new ApiKeyClearanceProperties();
    }

    @Override
    protected boolean shouldNotFilter(HttpServletRequest request) {
        String path = request.getRequestURI();
        // Allow WebSocket handshake, error dispatches, static assets, actuator, and API docs
        return path.startsWith("/ws")
                || path.startsWith("/actuator")
                || path.startsWith("/v3/api-docs")
                || path.startsWith("/swagger-ui")
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

        String apiKey = ApiKeyClearanceProperties.normalizeKey(request.getHeader(API_KEY_HEADER));
        ContextClearance clearance = null;
        if (apiKey != null && !apiKey.isEmpty()) {
            clearance = apiKeys.resolve(apiKey);
            if (clearance == null) {
                sendErrorResponse(response, HttpStatus.UNAUTHORIZED.value(), "Unauthorized", "Invalid API key.");
                return;
            }
        } else if (!apiKeys.isAllowUnsafeHeader()) {
            sendErrorResponse(response, HttpStatus.UNAUTHORIZED.value(), "Unauthorized", "API key required. Provide a valid " + API_KEY_HEADER + " header.");
            return;
        } else {
            clearance = ContextClearance.fromString(headerValue);
        }
        if (clearance == null) {
            sendForbiddenResponse(response, "Missing or invalid " + CLEARANCE_HEADER + " header. Allowed values: INNIE, OUTIE, SYSTEM, ADMIN.");
            return;
        }

        String traceId = request.getHeader(TRACE_HEADER);
        if (traceId == null || traceId.trim().isEmpty()) {
            traceId = java.util.UUID.randomUUID().toString();
        }
        request.setAttribute(TRACE_ATTRIBUTE, traceId);
        response.setHeader(TRACE_HEADER, traceId);

        ClearanceContext.setClearance(clearance);
        MDC.put("traceId", traceId);
        request.setAttribute(CLEARANCE_ATTRIBUTE, clearance);
        try {
            if (isDispatchRequest(request) && !allowDispatch(request)) {
                sendErrorResponse(response, 429, "Too Many Requests",
                        "Dispatch rate limit exceeded. Retry after 60 seconds.");
                return;
            }
            filterChain.doFilter(request, response);
        } finally {
            ClearanceContext.clear();
            MDC.remove("traceId");
        }
    }

    private boolean isDispatchRequest(HttpServletRequest request) {
        return "POST".equalsIgnoreCase(request.getMethod())
                && request.getRequestURI() != null
                && request.getRequestURI().endsWith("/compartments");
    }

    private boolean allowDispatch(HttpServletRequest request) {
        int limit = apiKeys.getDispatchPerMinute();
        if (limit <= 0) {
            return true;
        }
        String ip = request.getRemoteAddr() != null ? request.getRemoteAddr() : "unknown";
        if (dispatchWindows.size() > 10000) {
            dispatchWindows.clear();
        }
        long now = System.currentTimeMillis();
        long[] window = dispatchWindows.computeIfAbsent(ip, k -> new long[]{now, 0});
        synchronized (window) {
            if (now - window[0] > 60_000) {
                window[0] = now;
                window[1] = 0;
            }
            window[1]++;
            return window[1] <= limit;
        }
    }

    private void sendForbiddenResponse(HttpServletResponse response, String message) throws IOException {
        sendErrorResponse(response, HttpStatus.FORBIDDEN.value(), "Forbidden", message);
    }

    private void sendErrorResponse(HttpServletResponse response, int status, String error, String message) throws IOException {
        response.setStatus(status);
        response.setContentType(MediaType.APPLICATION_JSON_VALUE);
        Map<String, Object> body = Map.of(
                "status", status,
                "error", error,
                "message", message
        );
        response.getWriter().write(objectMapper.writeValueAsString(body));
    }
}
