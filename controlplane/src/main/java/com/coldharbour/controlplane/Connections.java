package com.coldharbour.controlplane;

import java.net.URI;

import javax.sql.DataSource;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.connection.lettuce.LettuceConnectionFactory;

import com.zaxxer.hikari.HikariDataSource;

@Configuration
public class Connections {

	@Bean
	public LettuceConnectionFactory redisConnectionFactory() {
		String addr = envOr("REDIS_ADDR", "127.0.0.1:6379");
		int split = addr.lastIndexOf(':');
		String host = split > 0 ? addr.substring(0, split) : addr;
		int port = split > 0 ? Integer.parseInt(addr.substring(split + 1)) : 6379;
		org.springframework.data.redis.connection.RedisStandaloneConfiguration standalone =
				new org.springframework.data.redis.connection.RedisStandaloneConfiguration(host, port);
		String password = System.getenv("REDIS_PASSWORD");
		if (password != null && !password.isBlank()) {
			standalone.setPassword(password);
		}
		if (!"1".equals(System.getenv("REDIS_TLS"))) {
			return new LettuceConnectionFactory(standalone);
		}
		org.springframework.data.redis.connection.lettuce.LettuceClientConfiguration client =
				org.springframework.data.redis.connection.lettuce.LettuceClientConfiguration.builder()
						.useSsl()
						.disablePeerVerification()
						.build();
		return new LettuceConnectionFactory(standalone, client);
	}

	@Bean
	public DataSource dataSource() {
		String dsn = System.getenv("POSTGRES_DSN");
		HikariDataSource ds = new HikariDataSource();
		if (dsn == null || dsn.isBlank()) {
			ds.setJdbcUrl("jdbc:postgresql://localhost:5433/coldharbour");
			ds.setUsername("coldharbour");
			ds.setPassword("coldharbour");
			ds.addDataSourceProperty("options", "-c TimeZone=UTC");
			return ds;
		}
		URI uri = URI.create(dsn);
		String userInfo = uri.getUserInfo();
		if (userInfo != null) {
			int colon = userInfo.indexOf(':');
			if (colon >= 0) {
				ds.setUsername(userInfo.substring(0, colon));
				ds.setPassword(userInfo.substring(colon + 1));
			} else {
				ds.setUsername(userInfo);
			}
		}
		String path = uri.getPath();
		if (path != null && path.startsWith("/")) {
			path = path.substring(1);
		}
		int port = uri.getPort() == -1 ? 5432 : uri.getPort();
		String jdbc = "jdbc:postgresql://" + uri.getHost() + ":" + port + "/" + path;
		if (uri.getQuery() != null && !uri.getQuery().isBlank()) {
			jdbc = jdbc + "?" + uri.getQuery();
		}
		ds.setJdbcUrl(jdbc);
		ds.addDataSourceProperty("options", "-c TimeZone=UTC");
		return ds;
	}

	private static String envOr(String key, String fallback) {
		String value = System.getenv(key);
		if (value == null || value.isBlank()) {
			return fallback;
		}
		return value;
	}
}
