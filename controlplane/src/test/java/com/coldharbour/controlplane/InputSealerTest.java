package com.coldharbour.controlplane;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.util.Arrays;
import java.util.UUID;

import org.junit.jupiter.api.Test;

class InputSealerTest {

	@Test
	void sealMatchesTheGoVector() {
		byte[] key = new byte[32];
		Arrays.fill(key, (byte) 1);
		byte[] nonce = new byte[12];
		Arrays.fill(nonce, (byte) 2);
		assertEquals("AgICAgICAgICAgICb7OlJSUCYwtZ7fA+dwycA4aL9zrt", InputSealer.sealWithNonce(key, nonce, "hello"));
	}

	@Test
	void durableIdMatchesTheGoVector() {
		assertEquals(UUID.fromString("fcf585ea-56d9-53a7-bd00-6e2560611dc2"), DurableID.forRedisJob("job-5"));
	}

	@Test
	void typeFileIncludesTheRegisteredTypes() {
		assertTrue(JobTypes.contains("redact"));
		assertTrue(JobTypes.contains("mask"));
	}
}
