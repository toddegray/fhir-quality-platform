package io.github.toddegray.fqp.core;

import org.springframework.http.HttpStatus;
import org.springframework.web.bind.annotation.ResponseStatus;

@ResponseStatus(HttpStatus.NOT_FOUND)
public class MeasureNotFoundException extends RuntimeException {
    public MeasureNotFoundException(String id) {
        super("Measure not found: " + id);
    }
}
