package com.coldharbour.controlplane;

import java.nio.ByteBuffer;
import java.security.MessageDigest;
import java.util.UUID;

final class DurableID {

	private static final UUID NAMESPACE = uuid5(uuidFrom("6ba7b810-9dad-11d1-80b4-00c04fd430c8"), "coldharbour.jobs");

	private DurableID() {
	}

	static UUID forRedisJob(String redisJobID) {
		return uuid5(NAMESPACE, redisJobID);
	}

	private static UUID uuid5(UUID namespace, String name) {
		try {
			MessageDigest sha1 = MessageDigest.getInstance("SHA-1");
			sha1.update(bytes(namespace));
			sha1.update(name.getBytes(java.nio.charset.StandardCharsets.UTF_8));
			byte[] sum = sha1.digest();
			sum[6] = (byte) ((sum[6] & 0x0f) | 0x50);
			sum[8] = (byte) ((sum[8] & 0x3f) | 0x80);
			ByteBuffer buf = ByteBuffer.wrap(sum);
			return new UUID(buf.getLong(), buf.getLong());
		} catch (Exception e) {
			throw new IllegalStateException("SHA-1 unavailable", e);
		}
	}

	private static UUID uuidFrom(String s) {
		return UUID.fromString(s);
	}

	private static byte[] bytes(UUID id) {
		ByteBuffer buf = ByteBuffer.allocate(16);
		buf.putLong(id.getMostSignificantBits());
		buf.putLong(id.getLeastSignificantBits());
		return buf.array();
	}
}
