import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

export interface GapPatient {
  patientId: string;
  age: number;
  latestHbA1c: number | null;
  latestHbA1cDate: string;
}

export interface MeasureResult {
  id: string;
  title: string;
  description: string;
  direction: string;
  reference: string;
  measurementPeriod: { start: string; end: string };
  denominator: number;
  numerator: number;
  percentage: number;
  gapPatients: GapPatient[];
}

@Injectable({ providedIn: 'root' })
export class MeasureService {
  private readonly http = inject(HttpClient);

  getCms122(): Observable<MeasureResult> {
    return this.http.get<MeasureResult>('/api/measures/cms122');
  }
}
