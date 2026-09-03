package com.coldharbor.controlplane.security;

/**
 * Thread-local holder for request clearance context.
 */
public final class ClearanceContext {

    private static final ThreadLocal<ContextClearance> CURRENT_CLEARANCE = new ThreadLocal<>();

    private ClearanceContext() {
    }

    public static void setClearance(ContextClearance clearance) {
        CURRENT_CLEARANCE.set(clearance);
    }

    public static ContextClearance getClearance() {
        return CURRENT_CLEARANCE.get();
    }

    public static void clear() {
        CURRENT_CLEARANCE.remove();
    }
}
