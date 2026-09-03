package com.coldharbor.controlplane.security;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.stereotype.Component;

import java.util.HashMap;
import java.util.Map;

/**
 * API-key to clearance mapping.
 *
 * <p>Keys are configured via {@code coldharbor.security.api-keys} (key -&gt; clearance).
 * When at least one key is configured, presenting a valid {@code X-API-Key} takes
 * precedence over the self-asserted clearance header. Set
 * {@code coldharbor.security.allow-unsafe-header=false} in production so header-only
 * access is rejected.
 */
@Component
@ConfigurationProperties(prefix = "coldharbor.security")
public class ApiKeyClearanceProperties {

    private Map<String, String> apiKeys = new HashMap<>();
    private boolean allowUnsafeHeader = true;

    public Map<String, String> getApiKeys() {
        return apiKeys;
    }

    public void setApiKeys(Map<String, String> apiKeys) {
        this.apiKeys = apiKeys != null ? apiKeys : new HashMap<>();
    }

    public boolean isAllowUnsafeHeader() {
        return allowUnsafeHeader;
    }

    public void setAllowUnsafeHeader(boolean allowUnsafeHeader) {
        this.allowUnsafeHeader = allowUnsafeHeader;
    }

    public ContextClearance resolve(String apiKey) {
        if (apiKey == null) {
            return null;
        }
        for (Map.Entry<String, String> entry : apiKeys.entrySet()) {
            if (constantTimeEquals(entry.getKey(), apiKey)) {
                return ContextClearance.fromString(entry.getValue());
            }
        }
        return null;
    }

    public boolean hasKeys() {
        return !apiKeys.isEmpty();
    }

    private static boolean constantTimeEquals(String a, String b) {
        if (a == null || b == null) {
            return false;
        }
        byte[] ab = a.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        byte[] bb = b.getBytes(java.nio.charset.StandardCharsets.UTF_8);
        return java.security.MessageDigest.isEqual(ab, bb);
    }

    public static String normalizeKey(String raw) {
        return raw != null ? raw.trim() : null;
    }
}
