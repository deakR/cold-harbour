package com.coldharbour.controlplane;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

import java.time.Duration;
import java.util.UUID;

import org.junit.jupiter.api.Test;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;

class PostLimitTest {

	@Test
	void twoLimitersShareOneRedisCounter() {
		StringRedisTemplate redis = mock(StringRedisTemplate.class);
		@SuppressWarnings("unchecked")
		ValueOperations<String, String> values = mock(ValueOperations.class);
		java.util.concurrent.atomic.AtomicLong next = new java.util.concurrent.atomic.AtomicLong();
		when(redis.opsForValue()).thenReturn(values);
		when(values.increment(anyString())).thenAnswer(invocation -> {
			String key = invocation.getArgument(0);
			assertTrue(key.startsWith("ratelimit:post:"));
			return next.incrementAndGet();
		});
		PostLimit first = new PostLimit(redis);
		PostLimit second = new PostLimit(redis);
		UUID tenant = UUID.randomUUID();
		for (int i = 0; i < PostLimit.MAX_PER_MINUTE; i++) {
			assertTrue(i % 2 == 0 ? first.allow(tenant) : second.allow(tenant));
		}
		assertFalse(first.allow(tenant));
		verify(redis).expire(anyString(), eq(Duration.ofSeconds(120)));
	}
}
