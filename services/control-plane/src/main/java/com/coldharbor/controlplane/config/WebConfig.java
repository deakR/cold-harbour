package com.coldharbor.controlplane.config;

import org.springframework.context.annotation.Configuration;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.web.servlet.config.annotation.CorsRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

/**
 * Web MVC and CORS configuration.
 */
@Configuration
public class WebConfig implements WebMvcConfigurer {

    @Value("${coldharbor.websocket.allowed-origins:http://localhost:3000,http://localhost:5173,http://127.0.0.1:3000,http://127.0.0.1:5173}")
    private String[] allowedOrigins;

    @Override
    public void addCorsMappings(CorsRegistry registry) {
        registry.addMapping("/**")
                .allowedOrigins(allowedOrigins)
                .allowedMethods("GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH")
                .allowedHeaders("*")
                .exposedHeaders("X-Context-Clearance", "X-Clearance",
                        "X-ColdHarbor-Context", "X-Trace-Id")
                .allowCredentials(false)
                .maxAge(3600);
    }

    @org.springframework.context.annotation.Bean
    @org.springframework.context.annotation.Primary
    public com.fasterxml.jackson.databind.ObjectMapper objectMapper() {
        com.fasterxml.jackson.databind.ObjectMapper mapper = new com.fasterxml.jackson.databind.ObjectMapper();
        mapper.registerModule(new com.fasterxml.jackson.datatype.jsr310.JavaTimeModule());
        mapper.disable(com.fasterxml.jackson.databind.SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);
        return mapper;
    }
}
