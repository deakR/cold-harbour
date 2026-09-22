package com.coldharbour.controlplane;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.verifyNoInteractions;
import static org.mockito.Mockito.when;

import java.util.Map;
import java.util.Optional;
import java.util.UUID;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentMatchers;
import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.data.redis.core.StreamOperations;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.jdbc.core.RowMapper;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import com.fasterxml.jackson.databind.ObjectMapper;

import jakarta.servlet.FilterChain;

class JobsControllerTest {

	private JdbcTemplate jdbc;
	private StringRedisTemplate redis;
	private JobsController jobs;
	private DeliveryController delivery;

	@BeforeEach
	void setUp() {
		jdbc = mock(JdbcTemplate.class);
		redis = mock(StringRedisTemplate.class);
		ObjectMapper mapper = new ObjectMapper();
		jobs = new JobsController(jdbc, redis, mapper);
		delivery = new DeliveryController(jdbc, mapper);
	}

	@Test
	void unknownJobTypeIs400AndWritesNothing() {
		MockHttpServletRequest req = tenantRequest(UUID.randomUUID());
		ResponseEntity<?> res = jobs.post(Map.of("input", "hello", "jobType", "nope"), req);
		assertEquals(400, res.getStatusCode().value());
		verifyNoInteractions(jdbc);
		verifyNoInteractions(redis);
	}

	@Test
	void otherTenantJobIs404() {
		when(jdbc.queryForObject(anyString(), ArgumentMatchers.<RowMapper<ResponseEntity<?>>>any(), any(), any(), any()))
				.thenThrow(new EmptyResultDataAccessException(1));
		when(jdbc.queryForObject(eq("SELECT tenant_id FROM outputs WHERE redis_job_id = ?"), eq(UUID.class), eq("job-a")))
				.thenReturn(UUID.randomUUID());
		ResponseEntity<?> res = jobs.get("job-a", tenantRequest(UUID.randomUUID()));
		assertEquals(404, res.getStatusCode().value());
	}

	@Test
	void otherTenantDeliveryLinkIs404() {
		ResponseEntity<?> res = delivery.create("job-a", Map.of("expiresAt", "2030-01-01T00:00:00Z", "maxViews", "1"),
				tenantRequest(UUID.randomUUID()));
		assertEquals(404, res.getStatusCode().value());
	}

	@Test
	void redisAddFailureDeletesTheAcceptRow() {
		@SuppressWarnings("unchecked")
		StreamOperations<String, Object, Object> ops = mock(StreamOperations.class);
		when(redis.opsForStream()).thenReturn(ops);
		when(ops.add(any())).thenThrow(new IllegalStateException("down"));
		ResponseEntity<?> res = jobs.post(Map.of("input", "hello"), tenantRequest(UUID.randomUUID()));
		assertEquals(503, res.getStatusCode().value());
		@SuppressWarnings("unchecked")
		Map<String, String> body = (Map<String, String>) res.getBody();
		assertFalse(body.containsKey("jobId"));
		verify(jdbc).update(eq("DELETE FROM job_accepts WHERE redis_job_id = ?"), any(Object.class));
	}

	@Test
	void missingAndRevokedKeysAre401() throws Exception {
		ApiKeys keys = mock(ApiKeys.class);
		when(keys.authenticate(null)).thenReturn(Optional.empty());
		when(keys.authenticate("gone")).thenReturn(Optional.empty());
		ApiKeyFilter filter = new ApiKeyFilter(keys);
		FilterChain chain = (request, response) -> {
			throw new AssertionError("filter passed the request");
		};
		MockHttpServletResponse missing = new MockHttpServletResponse();
		filter.doFilter(new MockHttpServletRequest(), missing, chain);
		assertEquals(401, missing.getStatus());
		MockHttpServletRequest revokedReq = new MockHttpServletRequest();
		revokedReq.addHeader("X-API-Key", "gone");
		MockHttpServletResponse revoked = new MockHttpServletResponse();
		filter.doFilter(revokedReq, revoked, chain);
		assertEquals(401, revoked.getStatus());
		verify(keys, never()).authenticate("ch_live_a_demo_key_aaaaaaaa");
	}

	private static MockHttpServletRequest tenantRequest(UUID tenantId) {
		MockHttpServletRequest req = new MockHttpServletRequest();
		req.setAttribute(ApiKeyFilter.ATTR_TENANT, tenantId);
		return req;
	}
}
