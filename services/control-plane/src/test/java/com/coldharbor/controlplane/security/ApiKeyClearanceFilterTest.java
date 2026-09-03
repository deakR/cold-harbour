package com.coldharbor.controlplane.security;

import jakarta.servlet.FilterChain;
import jakarta.servlet.ServletException;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import java.io.IOException;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

class ApiKeyClearanceFilterTest {

    private ContextClearanceFilter filterWithKeys(boolean allowUnsafe) {
        ApiKeyClearanceProperties props = new ApiKeyClearanceProperties();
        props.setApiKeys(Map.of("innie-key-1", "INNIE", "admin-key-1", "ADMIN"));
        props.setAllowUnsafeHeader(allowUnsafe);
        return new ContextClearanceFilter(props);
    }

    @Test
    @DisplayName("Valid API key grants mapped clearance without clearance header")
    void shouldGrantClearanceFromApiKey() throws ServletException, IOException {
        ContextClearanceFilter filter = filterWithKeys(false);
        MockHttpServletRequest request = new MockHttpServletRequest("POST", "/api/v1/compartments");
        request.addHeader("X-API-Key", "innie-key-1");
        MockHttpServletResponse response = new MockHttpServletResponse();
        FilterChain chain = mock(FilterChain.class);

        filter.doFilterInternal(request, response, chain);

        assertThat(response.getStatus()).isEqualTo(200);
        assertThat(request.getAttribute(ContextClearanceFilter.CLEARANCE_ATTRIBUTE)).isEqualTo(ContextClearance.INNIE);
        verify(chain, times(1)).doFilter(any(), any());
    }

    @Test
    @DisplayName("Unknown API key is rejected with 401")
    void shouldRejectUnknownApiKey() throws ServletException, IOException {
        ContextClearanceFilter filter = filterWithKeys(false);
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments");
        request.addHeader("X-API-Key", "wrong-key");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, mock(FilterChain.class));

        assertThat(response.getStatus()).isEqualTo(401);
    }

    @Test
    @DisplayName("Header-only access is rejected when keys are configured and unsafe header is disabled")
    void shouldRequireApiKeyWhenUnsafeDisabled() throws ServletException, IOException {
        ContextClearanceFilter filter = filterWithKeys(false);
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/compartments");
        request.addHeader("X-Context-Clearance", "INNIE");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, mock(FilterChain.class));

        assertThat(response.getStatus()).isEqualTo(401);
    }

    @Test
    @DisplayName("Trace ID is propagated or generated per request")
    void shouldPropagateTraceId() throws ServletException, IOException {
        ContextClearanceFilter filter = filterWithKeys(true);
        MockHttpServletRequest request = new MockHttpServletRequest("GET", "/api/v1/workers");
        request.addHeader("X-Context-Clearance", "ADMIN");
        request.addHeader("X-Trace-Id", "trace-123");
        MockHttpServletResponse response = new MockHttpServletResponse();

        filter.doFilterInternal(request, response, mock(FilterChain.class));

        assertThat(request.getAttribute(ContextClearanceFilter.TRACE_ATTRIBUTE)).isEqualTo("trace-123");
        assertThat(response.getHeader("X-Trace-Id")).isEqualTo("trace-123");
    }
}
