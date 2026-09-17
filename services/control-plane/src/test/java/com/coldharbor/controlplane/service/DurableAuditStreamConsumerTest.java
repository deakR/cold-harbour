package com.coldharbor.controlplane.service;

import com.coldharbor.controlplane.model.DeadDropPayload;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.Test;
import org.springframework.data.redis.connection.stream.MapRecord;
import org.springframework.data.redis.connection.stream.RecordId;
import org.springframework.data.redis.connection.stream.Consumer;
import org.springframework.data.redis.connection.stream.StreamOffset;
import org.springframework.data.redis.connection.stream.StreamReadOptions;
import org.springframework.data.redis.core.StreamOperations;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.test.util.ReflectionTestUtils;

import java.util.List;
import java.util.Map;

import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

class DurableAuditStreamConsumerTest {

    @Test
    @SuppressWarnings({"unchecked", "rawtypes"})
    void acknowledgesOnlyAfterAuditPersistence() throws Exception {
        StringRedisTemplate redisTemplate = mock(StringRedisTemplate.class);
        StreamOperations<String, Object, Object> operations = mock(StreamOperations.class);
        when(redisTemplate.opsForStream()).thenReturn((StreamOperations) operations);
        AuditService auditService = mock(AuditService.class);
        ObjectMapper objectMapper = new ObjectMapper();

        DeadDropPayload payload = new DeadDropPayload();
        payload.setCompartmentId("cpt_durable");
        payload.setOwnerId("owner");
        payload.setContext("OUTIE");
        payload.setTaskType("TASK");

        MapRecord<String, Object, Object> record = mock(MapRecord.class);
        when(record.getId()).thenReturn(RecordId.of("1-0"));
        when(record.getValue()).thenReturn(Map.of(
                "data", objectMapper.writeValueAsString(payload),
                "finalState", "FAILED",
                "details", "durable failure"));
        doReturn(List.of(record)).when(operations).read(
                any(Consumer.class),
                any(StreamReadOptions.class),
                any(StreamOffset[].class));

        DurableAuditStreamConsumer consumer =
                new DurableAuditStreamConsumer(redisTemplate, auditService, objectMapper);
        ReflectionTestUtils.setField(consumer, "streamName", "coldharbor:audits");
        ReflectionTestUtils.setField(consumer, "consumerGroup", "control-plane-audit");
        ReflectionTestUtils.setField(consumer, "consumerName", "audit-writer");
        ReflectionTestUtils.setField(consumer, "groupReady", true);

        consumer.poll();

        verify(auditService).recordCompletedJob(
                eq("cpt_durable"), any(DeadDropPayload.class),
                eq("durable failure"), eq("FAILED"));
        verify(operations).acknowledge(
                "coldharbor:audits", "control-plane-audit", RecordId.of("1-0"));
        verify(operations).delete("coldharbor:audits", RecordId.of("1-0"));
    }
}
