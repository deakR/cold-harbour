package com.coldharbour.controlplane;

import java.time.Duration;
import java.time.Instant;
import java.util.UUID;

import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Component;

@Component
public class PostLimit {

	static final int MAX_PER_MINUTE = 30;

	private final StringRedisTemplate redis;

	public PostLimit(StringRedisTemplate redis) {
		this.redis = redis;
	}

	public boolean allow(UUID tenantId) {
		long window = Instant.now().getEpochSecond() / 60;
		String key = "ratelimit:post:" + tenantId + ":" + window;
		Long count = redis.opsForValue().increment(key);
		if (count != null && count == 1L) {
			redis.expire(key, Duration.ofSeconds(120));
		}
		return count != null && count <= MAX_PER_MINUTE;
	}
}
