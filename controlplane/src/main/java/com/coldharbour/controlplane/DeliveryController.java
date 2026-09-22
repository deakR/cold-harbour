package com.coldharbour.controlplane;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Instant;
import java.util.HexFormat;
import java.util.Map;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RestController;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

import jakarta.servlet.http.HttpServletRequest;

@RestController
public class DeliveryController {

	private final JdbcTemplate jdbc;
	private final ObjectMapper mapper;
	private final SecureRandom random = new SecureRandom();

	public DeliveryController(JdbcTemplate jdbc, ObjectMapper mapper) {
		this.jdbc = jdbc;
		this.mapper = mapper;
	}

	@PostMapping("/jobs/{id}/delivery-links")
	public ResponseEntity<?> create(@PathVariable String id, @RequestBody Map<String, String> body,
			HttpServletRequest req) {
		UUID tenantId = (UUID) req.getAttribute(ApiKeyFilter.ATTR_TENANT);
		if (!owns(id, tenantId)) {
			return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
		}
		String expiresRaw = body == null ? null : body.get("expiresAt");
		String viewsRaw = body == null ? null : body.get("maxViews");
		if (expiresRaw == null || viewsRaw == null) {
			return ResponseEntity.badRequest().body(Map.of("error", "expiresAt and maxViews are required"));
		}
		Instant expiresAt = Instant.parse(expiresRaw);
		int maxViews = Integer.parseInt(viewsRaw);
		if (maxViews < 1) {
			return ResponseEntity.badRequest().body(Map.of("error", "maxViews must be at least 1"));
		}
		byte[] raw = new byte[32];
		random.nextBytes(raw);
		String token = HexFormat.of().formatHex(raw);
		jdbc.update(
				"INSERT INTO delivery_links (token_hash, redis_job_id, tenant_id, expires_at, max_views) VALUES (?, ?, ?, ?, ?)",
				sha256(token), id, tenantId, java.sql.Timestamp.from(expiresAt), maxViews);
		return ResponseEntity.ok(Map.of("token", token, "expiresAt", expiresRaw, "maxViews", maxViews));
	}

	@GetMapping("/d/{token}")
	public ResponseEntity<?> open(@PathVariable String token) {
		String hash = sha256(token);
		String jobId;
		try {
			jobId = jdbc.queryForObject("""
					UPDATE delivery_links
					SET view_count = view_count + 1
					WHERE token_hash = ?
					  AND view_count < max_views
					  AND expires_at > now()
					RETURNING redis_job_id
					""", String.class, hash);
		} catch (EmptyResultDataAccessException ignored) {
			return refused(hash);
		}
		jdbc.update("INSERT INTO delivery_link_accesses (token_hash) VALUES (?)", hash);
		Map<String, Object> row = jdbc.queryForMap(
				"SELECT o.tenant_id, o.body, j.output_signature, j.signing_key_id FROM outputs o JOIN jobs j ON j.tenant_id = o.tenant_id AND j.id = ? WHERE o.redis_job_id = ?",
				DurableID.forRedisJob(jobId), jobId);
		JsonNode result = readBody((String) row.get("body"));
		String signature = "";
		byte[] sig = (byte[]) row.get("output_signature");
		if (sig != null) {
			signature = java.util.Base64.getEncoder().encodeToString(sig);
		}
		Object keyId = row.get("signing_key_id");
		Object tenant = row.get("tenant_id");
		return ResponseEntity.ok(Map.of(
				"result", result,
				"tenantId", tenant == null ? "" : tenant.toString(),
				"signature", signature,
				"signingKeyId", keyId == null ? "" : keyId.toString()));
	}

	private ResponseEntity<?> refused(String hash) {
		try {
			Map<String, Object> row = jdbc.queryForMap(
					"SELECT expires_at > now() AS live, view_count, max_views FROM delivery_links WHERE token_hash = ?",
					hash);
			boolean live = Boolean.TRUE.equals(row.get("live"));
			int views = ((Number) row.get("view_count")).intValue();
			int max = ((Number) row.get("max_views")).intValue();
			if (!live) {
				return ResponseEntity.status(HttpStatus.GONE).build();
			}
			if (views >= max) {
				return ResponseEntity.status(HttpStatus.FORBIDDEN).build();
			}
		} catch (EmptyResultDataAccessException ignored) {
			return ResponseEntity.status(HttpStatus.NOT_FOUND).build();
		}
		return ResponseEntity.status(HttpStatus.FORBIDDEN).build();
	}

	private boolean owns(String jobId, UUID tenantId) {
		Integer owned = jdbc.query(
				"SELECT 1 FROM job_accepts WHERE redis_job_id = ? AND tenant_id = ?",
				rs -> rs.next() ? 1 : null,
				jobId, tenantId);
		if (owned != null) {
			return true;
		}
		return false;
	}

	private JsonNode readBody(String body) {
		try {
			return mapper.readTree(body);
		} catch (Exception e) {
			throw new IllegalStateException("outputs.body is not JSON", e);
		}
	}

	static String sha256(String raw) {
		try {
			MessageDigest digest = MessageDigest.getInstance("SHA-256");
			return HexFormat.of().formatHex(digest.digest(raw.getBytes(StandardCharsets.UTF_8)));
		} catch (Exception e) {
			throw new IllegalStateException("SHA-256 unavailable", e);
		}
	}
}
