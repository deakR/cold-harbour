package com.coldharbour.controlplane;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;
import java.util.Optional;
import java.util.UUID;

import org.springframework.dao.EmptyResultDataAccessException;
import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Component;

@Component
public final class ApiKeys {

	public record Principal(UUID tenantId) {
	}

	private final JdbcTemplate jdbc;

	public ApiKeys(JdbcTemplate jdbc) {
		this.jdbc = jdbc;
	}

	public Optional<Principal> authenticate(String raw) {
		if (raw == null || raw.isEmpty()) {
			return Optional.empty();
		}
		String hash = sha256Hex(raw);
		try {
			UUID tenantId = jdbc.queryForObject(
					"SELECT tenant_id FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL",
					UUID.class,
					hash
			);
			if (tenantId == null) {
				return Optional.empty();
			}
			return Optional.of(new Principal(tenantId));
		} catch (EmptyResultDataAccessException e) {
			return Optional.empty();
		}
	}

	static String sha256Hex(String raw) {
		try {
			MessageDigest digest = MessageDigest.getInstance("SHA-256");
			byte[] hashed = digest.digest(raw.getBytes(StandardCharsets.UTF_8));
			return HexFormat.of().formatHex(hashed);
		} catch (NoSuchAlgorithmException e) {
			throw new IllegalStateException("SHA-256 unavailable", e);
		}
	}
}
