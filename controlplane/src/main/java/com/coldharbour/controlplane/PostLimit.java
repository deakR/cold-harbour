package com.coldharbour.controlplane;

import java.time.Instant;
import java.util.ArrayDeque;
import java.util.Deque;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;

final class PostLimit {

	static final int MAX_PER_MINUTE = 30;

	private static final Map<UUID, Deque<Instant>> HITS = new ConcurrentHashMap<>();

	private PostLimit() {
	}

	static boolean allow(UUID tenantId) {
		Instant now = Instant.now();
		Deque<Instant> hits = HITS.computeIfAbsent(tenantId, id -> new ArrayDeque<>());
		synchronized (hits) {
			Instant cutoff = now.minusSeconds(60);
			while (!hits.isEmpty() && hits.peekFirst().isBefore(cutoff)) {
				hits.removeFirst();
			}
			if (hits.size() >= MAX_PER_MINUTE) {
				return false;
			}
			hits.addLast(now);
			return true;
		}
	}
}
