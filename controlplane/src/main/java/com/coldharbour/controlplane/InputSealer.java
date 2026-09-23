package com.coldharbour.controlplane;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.time.Duration;
import java.util.Base64;

import javax.crypto.Cipher;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.SecretKeySpec;

import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.json.JsonMapper;

final class InputSealer {

	private static final int KEY_BYTES = 32;
	private static final int NONCE_BYTES = 12;
	private static final int TAG_BITS = 128;
	private static final SecureRandom RANDOM = new SecureRandom();
	private static final ObjectMapper MAPPER = JsonMapper.builder().build();

	private InputSealer() {
	}

	static byte[] newKey() {
		byte[] key = new byte[KEY_BYTES];
		RANDOM.nextBytes(key);
		return key;
	}

	static String seal(byte[] key, String plain) {
		byte[] nonce = new byte[NONCE_BYTES];
		RANDOM.nextBytes(nonce);
		return sealWithNonce(key, nonce, plain);
	}

	static String sealWithNonce(byte[] key, byte[] nonce, String plain) {
		try {
			Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
			cipher.init(Cipher.ENCRYPT_MODE, new SecretKeySpec(key, "AES"), new GCMParameterSpec(TAG_BITS, nonce));
			byte[] body = cipher.doFinal(plain.getBytes(StandardCharsets.UTF_8));
			byte[] out = new byte[nonce.length + body.length];
			System.arraycopy(nonce, 0, out, 0, nonce.length);
			System.arraycopy(body, 0, out, nonce.length, body.length);
			return Base64.getEncoder().encodeToString(out);
		} catch (Exception e) {
			throw new IllegalStateException("seal input", e);
		}
	}

	static byte[] wrap(byte[] key) {
		String addr = System.getenv("VAULT_ADDR");
		String token = System.getenv("VAULT_TOKEN");
		if (addr == null || addr.isEmpty() || token == null || token.isEmpty()) {
			return key;
		}
		try {
			String body = MAPPER.writeValueAsString(java.util.Map.of(
					"plaintext", Base64.getEncoder().encodeToString(key)));
			HttpRequest req = HttpRequest.newBuilder(URI.create(addr + "/v1/transit/encrypt/coldharbour"))
					.timeout(Duration.ofSeconds(5))
					.header("X-Vault-Token", token)
					.header("Content-Type", "application/json")
					.POST(HttpRequest.BodyPublishers.ofString(body))
					.build();
			HttpResponse<String> res = HttpClient.newHttpClient()
					.send(req, HttpResponse.BodyHandlers.ofString());
			if (res.statusCode() / 100 != 2) {
				throw new IllegalStateException("vault encrypt status " + res.statusCode());
			}
			JsonNode ciphertext = MAPPER.readTree(res.body()).path("data").path("ciphertext");
			if (ciphertext.asString().isEmpty()) {
				throw new IllegalStateException("vault encrypt returned an empty ciphertext");
			}
			return ciphertext.asString().getBytes(StandardCharsets.UTF_8);
		} catch (RuntimeException e) {
			throw e;
		} catch (Exception e) {
			throw new IllegalStateException("vault encrypt", e);
		}
	}
}
