package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.dto.CompartmentDetail;
import com.coldharbor.controlplane.dto.CreateCompartmentRequest;
import com.coldharbor.controlplane.dto.DeadDropResponse;
import com.coldharbor.controlplane.exception.NotFoundException;
import com.coldharbor.controlplane.security.ContextClearanceFilter;
import com.coldharbor.controlplane.service.CompartmentService;
import com.coldharbor.controlplane.service.JobDispatchService;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.context.annotation.Import;
import org.springframework.http.MediaType;
import org.springframework.test.web.servlet.MockMvc;

import java.time.Instant;
import java.util.Map;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.post;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(CompartmentController.class)
@Import({ContextClearanceFilter.class, GlobalExceptionHandler.class})
class CompartmentControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @Autowired
    private ObjectMapper objectMapper;

    @MockBean
    private JobDispatchService jobDispatchService;

    @MockBean
    private CompartmentService compartmentService;

    @Test
    @DisplayName("POST /api/v1/compartments creates compartment with 201 Created when clearance matches")
    void shouldCreateCompartmentWhenClearanceMatches() throws Exception {
        CreateCompartmentRequest req = new CreateCompartmentRequest("cpt_1", "INNIE", "usr_100", "DATA_REDUCTION", Map.of("count", 10));
        CompartmentDetail detail = new CompartmentDetail("cpt_1", "INNIE", "usr_100", "DATA_REDUCTION", "QUEUED", 0, Instant.now(), Instant.now(), "dispatched");

        when(jobDispatchService.dispatchJob(any(CreateCompartmentRequest.class))).thenReturn(detail);

        mockMvc.perform(post("/api/v1/compartments")
                        .header("X-Context-Clearance", "INNIE")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(req)))
                .andExpect(status().isCreated())
                .andExpect(jsonPath("$.compartmentId").value("cpt_1"))
                .andExpect(jsonPath("$.context").value("INNIE"))
                .andExpect(jsonPath("$.state").value("QUEUED"));
    }

    @Test
    @DisplayName("POST /api/v1/compartments rejects INNIE creating OUTIE compartment with 403 Forbidden")
    void shouldRejectMismatchedClearanceOnCreation() throws Exception {
        CreateCompartmentRequest req = new CreateCompartmentRequest("cpt_2", "OUTIE", "usr_200", "DATA_REDUCTION", Map.of());

        mockMvc.perform(post("/api/v1/compartments")
                        .header("X-Context-Clearance", "INNIE")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(req)))
                .andExpect(status().isForbidden())
                .andExpect(jsonPath("$.error").value("Forbidden"));
    }

    @Test
    @DisplayName("POST /api/v1/compartments rejects missing clearance header with 403 Forbidden")
    void shouldRejectMissingClearanceHeader() throws Exception {
        CreateCompartmentRequest req = new CreateCompartmentRequest("cpt_3", "INNIE", "usr_300", "DATA_REDUCTION", Map.of());

        mockMvc.perform(post("/api/v1/compartments")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(req)))
                .andExpect(status().isForbidden());
    }

    @Test
    @DisplayName("POST /api/v1/compartments rejects invalid payload with 400 Bad Request")
    void shouldRejectInvalidPayload() throws Exception {
        CreateCompartmentRequest req = new CreateCompartmentRequest(); // missing required fields

        mockMvc.perform(post("/api/v1/compartments")
                        .header("X-Context-Clearance", "INNIE")
                        .contentType(MediaType.APPLICATION_JSON)
                        .content(objectMapper.writeValueAsString(req)))
                .andExpect(status().isBadRequest());
    }

    @Test
    @DisplayName("GET /api/v1/compartments/{id} returns compartment details")
    void shouldGetCompartmentDetails() throws Exception {
        CompartmentDetail detail = new CompartmentDetail("cpt_1", "INNIE", "usr_100", "DATA_REDUCTION", "RUNNING", 50, Instant.now(), Instant.now(), "In progress");
        when(compartmentService.getCompartment(eq("cpt_1"), any())).thenReturn(detail);

        mockMvc.perform(get("/api/v1/compartments/cpt_1")
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.compartmentId").value("cpt_1"))
                .andExpect(jsonPath("$.progress").value(50));
    }

    @Test
    @DisplayName("GET /api/v1/compartments/{id} returns 404 when compartment not found")
    void shouldReturn404WhenNotFound() throws Exception {
        when(compartmentService.getCompartment(eq("cpt_missing"), any()))
                .thenThrow(new NotFoundException("Compartment not found: cpt_missing"));

        mockMvc.perform(get("/api/v1/compartments/cpt_missing")
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isNotFound())
                .andExpect(jsonPath("$.error").value("Not Found"));
    }

    @Test
    @DisplayName("GET /api/v1/compartments/{id}/deaddrop returns dead drop archive")
    void shouldGetDeadDrop() throws Exception {
        DeadDropResponse resp = new DeadDropResponse();
        resp.setCompartmentId("cpt_1");
        resp.setContext("INNIE");
        resp.setChecksum("sha256_dead_drop_hash");
        resp.setDurationMs(800L);
        resp.setRemainingTtlSeconds(3500L);

        when(compartmentService.getDeadDrop(eq("cpt_1"), any())).thenReturn(resp);

        mockMvc.perform(get("/api/v1/compartments/cpt_1/deaddrop")
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.compartmentId").value("cpt_1"))
                .andExpect(jsonPath("$.checksum").value("sha256_dead_drop_hash"))
                .andExpect(jsonPath("$.remainingTtlSeconds").value(3500));
    }
}
