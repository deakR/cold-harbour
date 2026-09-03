package com.coldharbor.controlplane.security;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import java.io.IOException;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

class ContextClearanceFilterTest {

    private ContextClearanceFilter filter;
    private FilterChain filterChain;

    @BeforeEach
    void setUp() {
        filter = new ContextClearanceFilter();
        filterChain = mock(FilterChain.class);
    }

    @Test
    @DisplayName("Bypasses filter for /ws, /actuator, and /error endpoints")
    void shouldBypassUnprotectedPaths() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/ws/events");
        MockHttpServletResponse response = new MockHttpServletResponse();

        assertThat(filter.shouldNotFilter(request)).isTrue();

        request.setRequestURI("/actuator/health");
        assertThat(filter.shouldNotFilter(request)).isTrue();

        request.setRequestURI("/error");
        assertThat(filter.shouldNotFilter(request)).isTrue();

        request.setRequestURI("/api/v1/compartments");
        assertThat(filter.shouldNotFilter(request)).isFalse();
    }

    @Test
    @DisplayName("Returns 403 Forbidden when clearance header is missing on protected endpoint")
    void shouldRejectMissingClearanceHeader() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);

        assertThat(response.getStatus()).isEqualTo(403);
        assertThat(response.getContentAsString()).contains("Missing or invalid X-Context-Clearance header");
        verify(filterChain, never()).doFilter(any(), any());
    }

    @Test
    @DisplayName("Returns 403 Forbidden when clearance header contains invalid value")
    void shouldRejectInvalidClearanceHeader() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments");
        request.addHeader("X-Context-Clearance", "ROOT");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);

        assertThat(response.getStatus()).isEqualTo(403);
        verify(filterChain, never()).doFilter(any(), any());
    }

    @Test
    @DisplayName("Allows request with valid INNIE clearance header and sets context")
    void shouldAllowInnieClearance() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("POST", "/api/v1/compartments");
        request.addHeader("X-Context-Clearance", "INNIE");
        MockHttpServletResponse response = new MockHttpServletResponse();

        doAnswer(invocation -> {
            assertThat(ClearanceContext.getClearance()).isEqualTo(ContextClearance.INNIE);
            assertThat(request.getAttribute(ContextClearanceFilter.CLEARANCE_ATTRIBUTE)).isEqualTo(ContextClearance.INNIE);
            return null;
        }).when(filterChain).doFilter(any(), any());

        filter.doFilterInternal(request, response, filterChain);

        assertThat(response.getStatus()).isEqualTo(200);
        verify(filterChain, times(1)).doFilter(any(), any());
        assertThat(ClearanceContext.getClearance()).isNull(); // Cleared in finally
    }

    @Test
    @DisplayName("Allows request with valid OUTIE clearance header")
    void shouldAllowOutieClearance() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments/cpt_123");
        request.addHeader("X-Context-Clearance", "OUTIE");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);

        assertThat(response.getStatus()).isEqualTo(200);
        verify(filterChain, times(1)).doFilter(any(), any());
    }

    @Test
    @DisplayName("Allows request with SYSTEM and ADMIN clearances")
    void shouldAllowSystemAndAdminClearance() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/audits");
        request.addHeader("X-Context-Clearance", "SYSTEM");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);
        assertThat(response.getStatus()).isEqualTo(200);

        request = new MockHttpServletRequest("GET", "/api/v1/workers");
        request.addHeader("X-Context-Clearance", "ADMIN");
        response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);
        assertThat(response.getStatus()).isEqualTo(200);
    }

    @Test
    @DisplayName("Accepts fallback header X-Clearance if X-Context-Clearance is absent")
    void shouldAcceptFallbackClearanceHeader() throws ServletException, IOException {
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments");
        request.addHeader("X-Clearance", "INNIE");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, filterChain);

        assertThat(response.getStatus()).isEqualTo(200);
        verify(filterChain, times(1)).doFilter(any(), any());
    }
}
