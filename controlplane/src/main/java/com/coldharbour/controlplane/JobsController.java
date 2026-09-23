package com.coldharbour.controlplane;

import java.util.HashMap;
import java.util.Map;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.RecordId;
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
	private final PostLimit limit;

	public JobsController(JdbcTemplate jdbc, StringRedisTemplate redis, ObjectMapper mapper, PostLimit limit) {
		this.jdbc = jdbc;
		this.redis = redis;
		this.mapper = mapper;
		this.limit = limit;
	}

	@PostMapping("/jobs")
	public ResponseEntity<?> post(@RequestBody Map<String, String> body, HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);
		String input = body == null ? null : body.get("input");
		if (input == null || input.isEmpty()) {
			return ResponseEntity.badRequest().body(Map.of("error", "input is required"));
		}
		String jobType = body.get("jobType");
		if (jobType != null && !jobType.isEmpty() && !JobTypes.contains(jobType)) {
			return ResponseEntity.badRequest().body(Map.of("error", "unknown jobType"));
		}
		if (!limit.allow(tenantId)) {
			return ResponseEntity.status(HttpStatus.TOO_MANY_REQUESTS).build();
		}
		String jobId = UUID.randomUUID().toString();
		jdbc.update(
				"INSERT INTO job_accepts (redis_job_id, tenant_id, accepted_at) VALUES (?, ?, now())",
				jobId, tenantId
		);
		String sealed;
		try {
			byte[] key = InputSealer.newKey();
			sealed = InputSealer.seal(key, input);
			jdbc.update(
					"INSERT INTO job_keys (job_id, key_material) VALUES (?, ?)",
					DurableID.forRedisJob(jobId), InputSealer.wrap(key)
			);
		} catch (RuntimeException ex) {
			forget(jobId);
			return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(Map.of("error", "queue unavailable"));
		}
		Map<String, String> fields = new HashMap<>();
		fields.put("id", jobId);
		fields.put("input", sealed);
		fields.put("tenant_id", tenantId.toString());
		if (jobType != null && !jobType.isEmpty()) {
			fields.put("job_type", jobType);
		}
		MapRecord<String, String, String> record = StreamRecords.string(fields).withStreamKey(JOBS_STREAM);
		RecordId added;
		try {
			added = redis.opsForStream().add(record);
		} catch (RuntimeException ex) {
			forget(jobId);
			return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(Map.of("error", "queue unavailable"));
		}
		if (added == null) {
			forget(jobId);
			return ResponseEntity.status(HttpStatus.SERVICE_UNAVAILABLE).body(Map.of("error", "queue unavailable"));
		}
		return ResponseEntity.ok(Map.of("jobId", jobId, "status", "QUEUED"));
	}

	private void forget(String jobId) {
		jdbc.update("DELETE FROM job_keys WHERE job_id = ?", DurableID.forRedisJob(jobId));
		jdbc.update("DELETE FROM job_accepts WHERE redis_job_id = ?", jobId);
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
					""", (rs, rowNum) -> terminal(id, tenantId, rs.getString("final_state"), rs.getString("body"),
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
			return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
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
			return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
		}

		Object step = redis.opsForHash().get("job:" + id + ":mem", "step");
		if (step != null && !step.toString().isEmpty()) {
			return ResponseEntity.ok(Map.of("jobId", id, "status", "RUNNING"));
		}
		return ResponseEntity.ok(Map.of("jobId", id, "status", "QUEUED"));
	}

	private ResponseEntity<?> terminal(String id, UUID tenantID, String finalState, String body, byte[] signature, String signingKeyId) {
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
				"tenantId", tenantID.toString(),
				"signature", sig,
				"signingKeyId", signingKeyId == null ? "" : signingKeyId
		));
	}
}
