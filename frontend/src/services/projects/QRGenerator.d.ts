export interface QRCodeMatrix {
  readonly size: number;
  isDark(x: number, y: number): boolean;
}

export declare class QRGenerator {
  createMatrix(value: string): QRCodeMatrix;
  createDataUrl(value: string, width: number): Promise<string>;
}
