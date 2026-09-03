package com.coldharbor.controlplane.config;

import org.mockito.Mockito;
import org.springframework.boot.test.context.TestConfiguration;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Primary;
import org.springframework.data.redis.connection.RedisConnection;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.data.redis.core.RedisTemplate;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;
import org.springframework.data.redis.listener.RedisMessageListenerContainer;

import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

@TestConfiguration
public class TestRedisConfig {

    @Bean
    @Primary
    public RedisConnectionFactory testRedisConnectionFactory() {
        RedisConnectionFactory factory = mock(RedisConnectionFactory.class);
        RedisConnection connection = mock(RedisConnection.class);
        when(factory.getConnection()).thenReturn(connection);
        return factory;
    }

    @Bean
    @Primary
    public StringRedisTemplate testStringRedisTemplate(RedisConnectionFactory factory) {
        StringRedisTemplate template = mock(StringRedisTemplate.class);
        ValueOperations<String, String> valOps = mock(ValueOperations.class);
        when(template.opsForValue()).thenReturn(valOps);
        return template;
    }

    @Bean
    @Primary
    public RedisTemplate<String, Object> testRedisTemplate(RedisConnectionFactory factory) {
        return mock(RedisTemplate.class);
    }

    @Bean
    @Primary
    public RedisMessageListenerContainer testRedisMessageListenerContainer(RedisConnectionFactory factory) {
        RedisMessageListenerContainer container = new RedisMessageListenerContainer() {
            @Override
            public boolean isAutoStartup() {
                return false;
            }

            @Override
            public void start() {
                // no-op for test
            }

            @Override
            public void stop() {
                // no-op for test
            }

            @Override
            public boolean isRunning() {
                return false;
            }
        };
        container.setConnectionFactory(factory);
        return container;
    }
}
