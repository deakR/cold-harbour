package com.coldharbor.controlplane.repository;

import com.coldharbor.controlplane.entity.AuditRecord;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.autoconfigure.orm.jpa.DataJpaTest;
import org.springframework.test.context.ActiveProfiles;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.UUID;

import static org.assertj.core.api.Assertions.assertThat;

@DataJpaTest
@ActiveProfiles("test")
class AuditRecordRepositoryTest {

    @Autowired
    private AuditRecordRepository repository;

    @Test
    @DisplayName("Successfully persists and queries audit record by compartmentId and context")
    void shouldPersistAndQueryAuditRecord() {
        AuditRecord record = new AuditRecord(
                UUID.randomUUID(),
                "cpt_test_001",
                "usr_test_100",
                "INNIE",
                "DATA_REDUCTION",
                "PURGED",
                "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
                450L,
                Instant.now().minusSeconds(10),
                Instant.now(),
                "{\"result\":\"OK\"}"
        );

        AuditRecord saved = repository.save(record);
        assertThat(saved.getId()).isNotNull();

        List<AuditRecord> list = repository.findByCompartmentId("cpt_test_001");
        assertThat(list).hasSize(1);
        assertThat(list.get(0).getOwnerId()).isEqualTo("usr_test_100");
        assertThat(list.get(0).getContext()).isEqualTo("INNIE");
        assertThat(list.get(0).getDurationMs()).isEqualTo(450L);

        List<AuditRecord> innieList = repository.findByContext("INNIE");
        assertThat(innieList).isNotEmpty();

        boolean exists = repository.existsByCompartmentIdAndFinalState("cpt_test_001", "PURGED");
        assertThat(exists).isTrue();

        Optional<AuditRecord> latest = repository.findFirstByCompartmentIdOrderByCompletedAtDesc("cpt_test_001");
        assertThat(latest).isPresent();
        assertThat(latest.get().getChecksum()).isEqualTo("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
    }
}
