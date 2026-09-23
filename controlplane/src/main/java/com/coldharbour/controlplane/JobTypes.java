package com.coldharbour.controlplane;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.util.Collections;
import java.util.HashSet;
import java.util.Set;

final class JobTypes {

	private static final Set<String> ALL = load();

	private JobTypes() {
	}

	static boolean contains(String jobType) {
		return ALL.contains(jobType);
	}

	static Set<String> all() {
		return ALL;
	}

	private static Set<String> load() {
		InputStream in = JobTypes.class.getClassLoader().getResourceAsStream("types.txt");
		if (in == null) {
			throw new IllegalStateException("types.txt is not on the classpath");
		}
		Set<String> types = new HashSet<>();
		try (BufferedReader reader = new BufferedReader(new InputStreamReader(in, StandardCharsets.UTF_8))) {
			String line;
			while ((line = reader.readLine()) != null) {
				line = line.trim();
				if (line.isEmpty() || line.startsWith("#")) {
					continue;
				}
				types.add(line);
			}
		} catch (java.io.IOException e) {
			throw new IllegalStateException("types.txt", e);
		}
		if (types.isEmpty()) {
			throw new IllegalStateException("types.txt is empty");
		}
		return Collections.unmodifiableSet(types);
	}
}
