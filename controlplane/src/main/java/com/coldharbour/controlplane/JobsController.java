package com.coldharbour.controlplane;

import java.util.Map;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.StreamRecords;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import jakarta.servlet.http.HttpServletRequest;

@RestController
public class JobsController {

	private static final String JOBS_STREAM = "coldharbour:jobs";

	private final JdbcTemplate jdbc;
	private final StringRedisTemplate redis;
	private final ObjectMapper mapper;

	public JobsController(JdbcTemplate jdbc, StringRedisTemplate redis, ObjectMapper mapper) {
		this.jdbc = jdbc;
		this.redis = redis;
		this.mapper = mapper;
	}

	@PostMapping("/jobs")
	public ResponseEntity<?> post(@RequestBody Map<String, String> body, HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);
		String input = body == null ? null : body.get("input");
		if (input == null || input.isEmpty()) {
			return ResponseEntity.badRequest().body(Map.of("error", "input is required"));
		}
		String jobId = UUID.randomUUID().toString();
		jdbc.update(
				"INSERT INTO job_accepts (redis_job_id, tenant_id, accepted_at) VALUES (?, ?, now())",
				jobId, tenantId
		);
		MapRecord<String, String, String> record = StreamRecords.string(Map.of(
				"id", jobId,
				"input", input,
				"tenant_id", tenantId.toString()
		)).withStreamKey(JOBS_STREAM);
		redis.opsForStream().add(record);
		return ResponseEntity.ok(Map.of("jobId", jobId, "status", "QUEUED"));
	}

	@GetMapping("/jobs/{id}")
	public ResponseEntity<?> get(@PathVariable String id, HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);

		try {
			return jdbc.queryForObject(
					"SELECT final_state, body FROM outputs WHERE redis_job_id = ? AND tenant_id = ?",
					(rs, rowNum) -> terminal(id, rs.getString("final_state"), rs.getString("body")),
					id, tenantId
			);
		} catch (EmptyResultDataAccessException ignored) {
		}

		try {
			jdbc.queryForObject(
					"SELECT tenant_id FROM outputs WHERE redis_job_id = ?",
					UUID.class,
					id
			);
			return ResponseEntity.status(HttpStatus.FORBIDDEN).build();
		} catch (EmptyResultDataAccessException ignored) {
		}

		UUID acceptTenant;
		try {
			acceptTenant = jdbc.queryForObject(
					"SELECT tenant_id FROM job_accepts WHERE redis_job_id = ?",
					UUID.class,
					id
			);
		} catch (EmptyResultDataAccessException ignored) {
			return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
		}
		if (acceptTenant == null || !tenantId.equals(acceptTenant)) {
			return ResponseEntity.status(HttpStatus.FORBIDDEN).build();
		}

		Object step = redis.opsForHash().get("job:" + id + ":mem", "step");
		if (step != null && !step.toString().isEmpty()) {
			return ResponseEntity.ok(Map.of("jobId", id, "status", "RUNNING"));
		}
		return ResponseEntity.ok(Map.of("jobId", id, "status", "QUEUED"));
	}

	private ResponseEntity<?> terminal(String id, String finalState, String body) {
		JsonNode result;
		try {
			result = mapper.readTree(body);
		} catch (JsonProcessingException e) {
			throw new IllegalStateException("outputs.body is not JSON", e);
		}
		return ResponseEntity.ok(Map.of(
				"jobId", id,
				"status", finalState,
				"result", result
		));
	}
}
