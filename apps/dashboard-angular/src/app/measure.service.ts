import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

export interface GapPatient {
  patientId: string;
  age: number;
  providerId: string | null;
  detail: string;
}

export interface ProviderRow {
  providerId: string;
  numerator: number;
  denominator: number;
  percentage: number;
}

export interface Measure {
  measureId: string;
  title: string;
  description: string;
  direction: 'lower-is-better' | 'higher-is-better';
  measurementPeriod: { start: string; end: string };
  denominator: number;
  numerator: number;
  percentage: number;
  gapPatients: GapPatient[];
  providerBreakdown: ProviderRow[];
  reference?: string;
  diagnosisCodes?: string[];
  labCodes?: string[];
}

export interface ScorecardCell {
  providerId: string;
  numerator: number;
  denominator: number;
  percentage: number;
}

export interface ScorecardRow {
  measureId: string;
  title: string;
  direction: 'lower-is-better' | 'higher-is-better';
  overallPercentage: number;
  overallNumerator: number;
  overallDenominator: number;
  cells: ScorecardCell[];
}

export interface Scorecard {
  providers: string[];
  measures: ScorecardRow[];
}

@Injectable({ providedIn: 'root' })
export class MeasureService {
  private readonly http = inject(HttpClient);

  listMeasures(): Observable<{ measures: Measure[] }> {
    return this.http.get<{ measures: Measure[] }>('/api/measures');
  }

  getMeasure(measureId: string): Observable<Measure> {
    return this.http.get<Measure>(`/api/measures/${measureId}`);
  }

  getScorecard(): Observable<Scorecard> {
    return this.http.get<Scorecard>('/api/scorecard');
  }

  getHistory(measureId: string): Observable<{ measureId: string; source: string; points: HistoryEntry[] }> {
    return this.http.get<{ measureId: string; source: string; points: HistoryEntry[] }>(
      `/api/measures/${measureId}/history`,
    );
  }
}

export interface HistoryEntry {
  periodEnd: string;
  percentage: number;
  numerator: number;
  denominator: number;
}
