package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.dto.WorkerStatusResponse;
import com.coldharbor.controlplane.security.ContextClearanceFilter;
import com.coldharbor.controlplane.service.WorkerWellnessMonitor;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.context.annotation.Import;
import org.springframework.test.web.servlet.MockMvc;

import java.time.Instant;
import java.util.List;
import java.util.Map;

import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(WorkerController.class)
@Import({ContextClearanceFilter.class, GlobalExceptionHandler.class})
class WorkerControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockBean
    private WorkerWellnessMonitor workerWellnessMonitor;

    @Test
    @DisplayName("GET /api/v1/workers returns list of active workers")
    void shouldReturnActiveWorkers() throws Exception {
        WorkerStatusResponse worker = new WorkerStatusResponse(
                "worker-go-01",
                "BUSY",
                "cpt_test_99",
                Instant.now(),
                true,
                2L
        );

        when(workerWellnessMonitor.getActiveWorkers()).thenReturn(List.of(worker));

        mockMvc.perform(get("/api/v1/workers")
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].workerId").value("worker-go-01"))
                .andExpect(jsonPath("$[0].status").value("BUSY"))
                .andExpect(jsonPath("$[0].healthy").value(true));
    }

    @Test
    @DisplayName("GET /api/v1/workers/dlq returns stream size and entries")
    void shouldReturnDlqSnapshot() throws Exception {
        when(workerWellnessMonitor.getDlqSnapshot(anyInt())).thenReturn(Map.of(
                "stream", "coldharbor:jobs:dlq",
                "size", 2,
                "entries", List.of(Map.of("_streamId", "1-0"))));

        mockMvc.perform(get("/api/v1/workers/dlq")
                        .header("X-Context-Clearance", "SYSTEM"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.stream").value("coldharbor:jobs:dlq"))
                .andExpect(jsonPath("$.size").value(2));
    }
}
