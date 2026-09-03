package com.coldharbor.controlplane.controller;

import com.coldharbor.controlplane.entity.AuditRecord;
import com.coldharbor.controlplane.security.ContextClearanceFilter;
import com.coldharbor.controlplane.service.AuditService;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.web.servlet.WebMvcTest;
import org.springframework.boot.test.mock.mockito.MockBean;
import org.springframework.context.annotation.Import;
import org.springframework.test.web.servlet.MockMvc;

import java.time.Instant;
import java.util.List;
import java.util.UUID;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.when;
import static org.springframework.test.web.servlet.request.MockMvcRequestBuilders.get;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.jsonPath;
import static org.springframework.test.web.servlet.result.MockMvcResultMatchers.status;

@WebMvcTest(AuditController.class)
@Import({ContextClearanceFilter.class, GlobalExceptionHandler.class})
class AuditControllerTest {

    @Autowired
    private MockMvc mockMvc;

    @MockBean
    private AuditService auditService;

    @Test
    @DisplayName("GET /api/v1/audits returns audit records matching clearance")
    void shouldReturnAudits() throws Exception {
        AuditRecord record = new AuditRecord(
                UUID.randomUUID(),
                "cpt_100",
                "usr_1",
                "INNIE",
                "DATA_REDUCTION",
                "PURGED",
                "checksum123",
                250L,
                Instant.now(),
                Instant.now(),
                "{}"
        );

        when(auditService.findAudits(any(), any(), any(), any())).thenReturn(List.of(record));

        mockMvc.perform(get("/api/v1/audits")
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$[0].compartmentId").value("cpt_100"))
                .andExpect(jsonPath("$[0].context").value("INNIE"))
                .andExpect(jsonPath("$[0].checksum").value("checksum123"));
    }

    @Test
    @DisplayName("GET /api/v1/audits/{id} returns specific audit record")
    void shouldReturnAuditById() throws Exception {
        UUID id = UUID.randomUUID();
        AuditRecord record = new AuditRecord(
                id,
                "cpt_100",
                "usr_1",
                "INNIE",
                "DATA_REDUCTION",
                "PURGED",
                "checksum123",
                250L,
                Instant.now(),
                Instant.now(),
                "{}"
        );

        when(auditService.findById(any(), any())).thenReturn(record);

        mockMvc.perform(get("/api/v1/audits/" + id)
                        .header("X-Context-Clearance", "INNIE"))
                .andExpect(status().isOk())
                .andExpect(jsonPath("$.id").value(id.toString()))
                .andExpect(jsonPath("$.compartmentId").value("cpt_100"));
    }
}
