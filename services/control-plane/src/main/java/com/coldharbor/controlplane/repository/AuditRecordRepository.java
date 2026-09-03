package com.coldharbor.controlplane.repository;

import com.coldharbor.controlplane.entity.AuditRecord;
import org.springframework.data.jpa.repository.JpaRepository;
import org.springframework.data.jpa.repository.JpaSpecificationExecutor;
import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Optional;
import java.util.UUID;

@Repository
public interface AuditRecordRepository extends JpaRepository<AuditRecord, UUID>, JpaSpecificationExecutor<AuditRecord> {

    List<AuditRecord> findByCompartmentId(String compartmentId);

    List<AuditRecord> findByOwnerId(String ownerId);

    List<AuditRecord> findByContext(String context);

    Optional<AuditRecord> findFirstByCompartmentIdOrderByCompletedAtDesc(String compartmentId);

    boolean existsByCompartmentIdAndFinalState(String compartmentId, String finalState);
}
