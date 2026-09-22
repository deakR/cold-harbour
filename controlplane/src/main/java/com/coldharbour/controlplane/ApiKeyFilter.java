package com.coldharbour.controlplane;

import java.io.IOException;
import java.util.Optional;

import org.springframework.stereotype.Component;
import org.springframework.web.filter.OncePerRequestFilter;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;

@Component
public final class ApiKeyFilter extends OncePerRequestFilter {

	public static final String ATTR_TENANT = "coldharbour.tenantId";

	private final ApiKeys apiKeys;

	public ApiKeyFilter(ApiKeys apiKeys) {
		this.apiKeys = apiKeys;
	}

	@Override
	protected boolean shouldNotFilter(HttpServletRequest request) {
		String path = request.getRequestURI();
		return path != null && path.startsWith("/d/");
	}

	@Override
	protected void doFilterInternal(HttpServletRequest request, HttpServletResponse response,
			FilterChain filterChain) throws ServletException, IOException {
		String raw = request.getHeader("X-API-Key");
		if ((raw == null || raw.isEmpty()) && request.getRequestURI() != null
				&& request.getRequestURI().startsWith("/ws/")) {
			raw = request.getParameter("apiKey");
		}
		Optional<ApiKeys.Principal> principal = apiKeys.authenticate(raw);
		if (principal.isEmpty()) {
			response.setStatus(HttpServletResponse.SC_UNAUTHORIZED);
			return;
		}
		request.setAttribute(ATTR_TENANT, principal.get().tenantId());
		filterChain.doFilter(request, response);
	}
}
