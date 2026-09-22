package com.coldharbour.controlplane;

import java.util.HashMap;
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
		Map<String, String> fields = new HashMap<>();
		fields.put("id", jobId);
		fields.put("input", input);
		fields.put("tenant_id", tenantId.toString());
		String jobType = body.get("jobType");
		if (jobType != null && !jobType.isEmpty()) {
			fields.put("job_type", jobType);
		}
		MapRecord<String, String, String> record = StreamRecords.string(fields).withStreamKey(JOBS_STREAM);
		redis.opsForStream().add(record);
		return ResponseEntity.ok(Map.of("jobId", jobId, "status", "QUEUED"));
	}

	@GetMapping("/jobs")
	public ResponseEntity<?> list(HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);
		var rows = jdbc.query("""
				SELECT a.redis_job_id, a.accepted_at, o.final_state
				FROM job_accepts a
				LEFT JOIN outputs o ON o.redis_job_id = a.redis_job_id AND o.tenant_id = a.tenant_id
				WHERE a.tenant_id = ?
				ORDER BY a.accepted_at DESC
				""", (rs, rowNum) -> {
			String state = rs.getString("final_state");
			if (state == null || state.isEmpty()) {
				state = "QUEUED";
			}
			return Map.of(
					"jobId", rs.getString("redis_job_id"),
					"acceptedAt", rs.getTimestamp("accepted_at").toInstant().toString(),
					"status", state);
		}, tenantId);
		return ResponseEntity.ok(rows);
	}

	@GetMapping("/jobs/{id}")
	public ResponseEntity<?> get(@PathVariable String id, HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);

		try {
			return jdbc.queryForObject("""
					SELECT o.final_state, o.body, j.output_signature, j.signing_key_id
					FROM outputs o
					LEFT JOIN jobs j ON j.id = ? AND j.tenant_id = o.tenant_id
					WHERE o.redis_job_id = ? AND o.tenant_id = ?
					""", (rs, rowNum) -> terminal(id, rs.getString("final_state"), rs.getString("body"),
					rs.getBytes("output_signature"), rs.getString("signing_key_id")),
					DurableID.forRedisJob(id), id, tenantId);
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

	private ResponseEntity<?> terminal(String id, String finalState, String body, byte[] signature, String signingKeyId) {
		JsonNode result;
		try {
			result = mapper.readTree(body);
		} catch (JsonProcessingException e) {
			throw new IllegalStateException("outputs.body is not JSON", e);
		}
		String sig = signature == null ? "" : java.util.Base64.getEncoder().encodeToString(signature);
		return ResponseEntity.ok(Map.of(
				"jobId", id,
				"status", finalState,
				"result", result,
				"signature", sig,
				"signingKeyId", signingKeyId == null ? "" : signingKeyId
		));
	}
}
