package com.coldharbor.controlplane.security;

import java.util.Locale;

/**
 * Context Clearance levels enforcing zero-trust namespace isolation.
 */
public enum ContextClearance {
    INNIE,
    OUTIE,
    SYSTEM,
    ADMIN;

    public static ContextClearance fromString(String value) {
        if (value == null || value.trim().isEmpty()) {
            return null;
        }
        try {
            return ContextClearance.valueOf(value.trim().toUpperCase(Locale.ROOT));
        } catch (IllegalArgumentException e) {
            return null;
        }
    }

    /**
     * Determines whether this caller clearance can access a target resource context.
     * - INNIE clearance can only access INNIE resources.
     * - OUTIE clearance can only access OUTIE resources.
     * - SYSTEM and ADMIN clearances can access any context.
     */
    public boolean canAccess(ContextClearance targetContext) {
        if (this == SYSTEM || this == ADMIN) {
            return true;
        }
        return this == targetContext;
    }

    public boolean canAccess(String targetContextStr) {
        ContextClearance target = fromString(targetContextStr);
        if (target == null) {
            return false;
        }
        return canAccess(target);
    }
}
