const ALPHANUMERIC = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:";
const MEDIUM_ECC_BYTES = [-1,10,16,26,18,24,16,18,22,22,26,30,22,22,24,24,28,28,26,26,26,26,28,28,28,28,28,28,28,28,28,28,28,28,28,28,28,28,28,28,28];
const MEDIUM_BLOCKS = [-1,1,1,1,2,2,4,4,4,5,5,5,8,9,9,10,10,11,13,14,16,17,17,18,20,21,23,25,26,28,29,31,33,35,37,38,40,43,45,47,49];
const MASKS = [
  (row, col) => (row + col) % 2 === 0,
  (row) => row % 2 === 0,
  (_row, col) => col % 3 === 0,
  (row, col) => (row + col) % 3 === 0,
  (row, col) => (Math.floor(row / 2) + Math.floor(col / 3)) % 2 === 0,
  (row, col) => row * col % 2 + row * col % 3 === 0,
  (row, col) => (row * col % 2 + row * col % 3) % 2 === 0,
  (row, col) => ((row + col) % 2 + row * col % 3) % 2 === 0,
];

class BitBuffer {
  bits = [];

  append(value, length) {
    for (let bit = length - 1; bit >= 0; bit--) this.bits.push(value >>> bit & 1);
  }

  toBytes() {
    const bytes = new Uint8Array(Math.ceil(this.bits.length / 8));
    this.bits.forEach((bit, index) => bytes[index >>> 3] |= bit << (7 - (index & 7)));
    return [...bytes];
  }
}

const gf = (() => {
  const exp = new Uint8Array(512);
  const log = new Uint8Array(256);
  let value = 1;
  for (let index = 0; index < 255; index++) {
    exp[index] = value;
    log[value] = index;
    value <<= 1;
    if (value & 0x100) value ^= 0x11d;
  }
  for (let index = 255; index < exp.length; index++) exp[index] = exp[index - 255];
  return { multiply: (left, right) => left && right ? exp[log[left] + log[right]] : 0 };
})();

function rawModuleCount(version) {
  let result = (16 * version + 128) * version + 64;
  if (version >= 2) {
    const alignmentCount = Math.floor(version / 7) + 2;
    result -= (25 * alignmentCount - 10) * alignmentCount - 55;
    if (version >= 7) result -= 36;
  }
  return result;
}

function dataCodewordCount(version) {
  return Math.floor(rawModuleCount(version) / 8)
    - MEDIUM_ECC_BYTES[version] * MEDIUM_BLOCKS[version];
}

function appendEncodedText(buffer, text, version) {
  if (/^[0-9]+$/.test(text)) {
    buffer.append(1, 4);
    buffer.append(text.length, version < 10 ? 10 : version < 27 ? 12 : 14);
    for (let index = 0; index < text.length; index += 3) {
      const group = text.slice(index, index + 3);
      buffer.append(Number(group), group.length * 3 + 1);
    }
    return;
  }

  if (text.length && [...text].every((character) => ALPHANUMERIC.includes(character))) {
    buffer.append(2, 4);
    buffer.append(text.length, version < 10 ? 9 : version < 27 ? 11 : 13);
    for (let index = 0; index < text.length; index += 2) {
      const first = ALPHANUMERIC.indexOf(text[index]);
      if (index + 1 < text.length) buffer.append(first * 45 + ALPHANUMERIC.indexOf(text[index + 1]), 11);
      else buffer.append(first, 6);
    }
    return;
  }

  const bytes = new TextEncoder().encode(text);
  buffer.append(4, 4);
  buffer.append(bytes.length, version < 10 ? 8 : 16);
  bytes.forEach((byte) => buffer.append(byte, 8));
}

function createDataCodewords(text, version) {
  const capacity = dataCodewordCount(version) * 8;
  const buffer = new BitBuffer();
  appendEncodedText(buffer, text, version);
  if (buffer.bits.length > capacity) return null;
  buffer.append(0, Math.min(4, capacity - buffer.bits.length));
  while (buffer.bits.length % 8) buffer.bits.push(0);
  let padding = 0xec;
  while (buffer.bits.length < capacity) {
    buffer.append(padding, 8);
    padding = padding === 0xec ? 0x11 : 0xec;
  }
  return buffer.toBytes();
}

function reedSolomonGenerator(degree) {
  const polynomial = new Uint8Array(degree);
  polynomial[degree - 1] = 1;
  let root = 1;
  for (let index = 0; index < degree; index++) {
    for (let term = 0; term < polynomial.length; term++) {
      polynomial[term] = gf.multiply(polynomial[term], root)
        ^ (term + 1 < polynomial.length ? polynomial[term + 1] : 0);
    }
    root = gf.multiply(root, 2);
  }
  return polynomial;
}

function reedSolomonRemainder(data, generator) {
  const result = new Uint8Array(generator.length);
  for (const byte of data) {
    const factor = byte ^ result[0];
    result.copyWithin(0, 1);
    result[result.length - 1] = 0;
    generator.forEach((coefficient, index) => result[index] ^= gf.multiply(coefficient, factor));
  }
  return [...result];
}

function addErrorCorrection(data, version) {
  const blockCount = MEDIUM_BLOCKS[version];
  const eccLength = MEDIUM_ECC_BYTES[version];
  const totalCodewords = Math.floor(rawModuleCount(version) / 8);
  const shortBlockCount = blockCount - totalCodewords % blockCount;
  const shortBlockLength = Math.floor(totalCodewords / blockCount);
  const generator = reedSolomonGenerator(eccLength);
  const blocks = [];
  let offset = 0;

  for (let index = 0; index < blockCount; index++) {
    const dataLength = shortBlockLength - eccLength + (index < shortBlockCount ? 0 : 1);
    const blockData = data.slice(offset, offset + dataLength);
    offset += dataLength;
    blocks.push({ data: blockData, ecc: reedSolomonRemainder(blockData, generator) });
  }

  const interleaved = [];
  const longestDataBlock = Math.max(...blocks.map((block) => block.data.length));
  for (let index = 0; index < longestDataBlock; index++) {
    blocks.forEach((block) => {
      if (index < block.data.length) interleaved.push(block.data[index]);
    });
  }
  for (let index = 0; index < eccLength; index++) {
    blocks.forEach((block) => interleaved.push(block.ecc[index]));
  }
  return interleaved;
}

function alignmentPositions(version) {
  if (version === 1) return [];
  const count = Math.floor(version / 7) + 2;
  const step = version === 32 ? 26 : Math.floor((version * 4 + count * 2 + 1) / (count * 2 - 2)) * 2;
  const positions = [6];
  for (let position = version * 4 + 10; positions.length < count; position -= step) positions.splice(1, 0, position);
  return positions;
}

function setFunction(matrix, reserved, row, col, dark) {
  if (row < 0 || col < 0 || row >= matrix.length || col >= matrix.length) return;
  matrix[row][col] = dark;
  reserved[row][col] = true;
}

function drawFinder(matrix, reserved, centerRow, centerCol) {
  for (let rowOffset = -4; rowOffset <= 4; rowOffset++) {
    for (let colOffset = -4; colOffset <= 4; colOffset++) {
      const distance = Math.max(Math.abs(rowOffset), Math.abs(colOffset));
      setFunction(matrix, reserved, centerRow + rowOffset, centerCol + colOffset, distance !== 2 && distance !== 4);
    }
  }
}

function drawAlignment(matrix, reserved, centerRow, centerCol) {
  for (let rowOffset = -2; rowOffset <= 2; rowOffset++) {
    for (let colOffset = -2; colOffset <= 2; colOffset++) {
      setFunction(matrix, reserved, centerRow + rowOffset, centerCol + colOffset,
        Math.max(Math.abs(rowOffset), Math.abs(colOffset)) !== 1);
    }
  }
}

function drawFormat(matrix, reserved, mask) {
  const size = matrix.length;
  const data = mask; // Medium error correction uses format bits 00.
  let remainder = data;
  for (let index = 0; index < 10; index++) remainder = remainder << 1 ^ (remainder >>> 9) * 0x537;
  const bits = (data << 10 | remainder) ^ 0x5412;
  const bit = (index) => (bits >>> index & 1) !== 0;

  for (let index = 0; index <= 5; index++) setFunction(matrix, reserved, index, 8, bit(index));
  setFunction(matrix, reserved, 7, 8, bit(6));
  setFunction(matrix, reserved, 8, 8, bit(7));
  setFunction(matrix, reserved, 8, 7, bit(8));
  for (let index = 9; index < 15; index++) setFunction(matrix, reserved, 8, 14 - index, bit(index));
  for (let index = 0; index < 8; index++) setFunction(matrix, reserved, 8, size - 1 - index, bit(index));
  for (let index = 8; index < 15; index++) setFunction(matrix, reserved, size - 15 + index, 8, bit(index));
  setFunction(matrix, reserved, size - 8, 8, true);
}

function drawVersion(matrix, reserved, version) {
  if (version < 7) return;
  let remainder = version;
  for (let index = 0; index < 12; index++) remainder = remainder << 1 ^ (remainder >>> 11) * 0x1f25;
  const bits = version << 12 | remainder;
  for (let index = 0; index < 18; index++) {
    const dark = (bits >>> index & 1) !== 0;
    const high = matrix.length - 11 + index % 3;
    const low = Math.floor(index / 3);
    setFunction(matrix, reserved, low, high, dark);
    setFunction(matrix, reserved, high, low, dark);
  }
}

function createFunctionMatrix(version) {
  const size = version * 4 + 17;
  const matrix = Array.from({ length: size }, () => Array(size).fill(false));
  const reserved = Array.from({ length: size }, () => Array(size).fill(false));
  for (let index = 0; index < size; index++) {
    setFunction(matrix, reserved, 6, index, index % 2 === 0);
    setFunction(matrix, reserved, index, 6, index % 2 === 0);
  }
  drawFinder(matrix, reserved, 3, 3);
  drawFinder(matrix, reserved, 3, size - 4);
  drawFinder(matrix, reserved, size - 4, 3);
  const positions = alignmentPositions(version);
  positions.forEach((row, rowIndex) => positions.forEach((col, colIndex) => {
    const last = positions.length - 1;
    if (!((rowIndex === 0 && colIndex === 0) || (rowIndex === 0 && colIndex === last) || (rowIndex === last && colIndex === 0))) {
      drawAlignment(matrix, reserved, row, col);
    }
  }));
  drawFormat(matrix, reserved, 0);
  drawVersion(matrix, reserved, version);
  return { matrix, reserved };
}

function placeData(matrix, reserved, codewords) {
  let bitIndex = 0;
  for (let right = matrix.length - 1; right >= 1; right -= 2) {
    if (right === 6) right = 5;
    for (let vertical = 0; vertical < matrix.length; vertical++) {
      const upward = ((right + 1) & 2) === 0;
      const row = upward ? matrix.length - 1 - vertical : vertical;
      for (let offset = 0; offset < 2; offset++) {
        const col = right - offset;
        if (!reserved[row][col] && bitIndex < codewords.length * 8) {
          matrix[row][col] = (codewords[bitIndex >>> 3] >>> (7 - (bitIndex & 7)) & 1) !== 0;
          bitIndex++;
        }
      }
    }
  }
}

function applyMask(matrix, reserved, mask) {
  return matrix.map((row, rowIndex) => row.map((dark, colIndex) =>
    !reserved[rowIndex][colIndex] && MASKS[mask](rowIndex, colIndex) ? !dark : dark));
}

function penaltyScore(matrix) {
  let score = 0;
  const lines = [...matrix, ...matrix[0].map((_, col) => matrix.map((row) => row[col]))];
  lines.forEach((line) => {
    let run = 1;
    for (let index = 1; index < line.length; index++) {
      run = line[index] === line[index - 1] ? run + 1 : 1;
      if (run === 5) score += 3;
      else if (run > 5) score++;
    }
    const text = line.map(Number).join("");
    for (let index = 0; index <= text.length - 11; index++) {
      const sample = text.slice(index, index + 11);
      if (sample === "00001011101" || sample === "10111010000") score += 40;
    }
  });
  for (let row = 0; row < matrix.length - 1; row++) {
    for (let col = 0; col < matrix.length - 1; col++) {
      const dark = matrix[row][col];
      if (matrix[row][col + 1] === dark && matrix[row + 1][col] === dark && matrix[row + 1][col + 1] === dark) score += 3;
    }
  }
  const darkCount = matrix.flat().filter(Boolean).length;
  score += Math.floor(Math.abs(darkCount * 100 / (matrix.length ** 2) - 50) / 5) * 10;
  return score;
}

function generateMatrix(text) {
  let version;
  let data;
  for (version = 1; version <= 40; version++) {
    data = createDataCodewords(text, version);
    if (data) break;
  }
  if (!data || version > 40) throw new RangeError("QR code data is too long");
  const codewords = addErrorCorrection(data, version);
  const base = createFunctionMatrix(version);
  placeData(base.matrix, base.reserved, codewords);

  let best;
  let bestScore = Infinity;
  for (let mask = 0; mask < MASKS.length; mask++) {
    const candidate = applyMask(base.matrix, base.reserved, mask);
    drawFormat(candidate, base.reserved.map((row) => [...row]), mask);
    const score = penaltyScore(candidate);
    if (score < bestScore) {
      best = candidate;
      bestScore = score;
    }
  }
  return best;
}

export class QRGenerator {
  createMatrix(value) {
    if (typeof value !== "string") throw new TypeError("QR code value must be a string");
    const modules = generateMatrix(value);
    return {
      size: modules.length,
      isDark: (x, y) => Number.isInteger(x) && Number.isInteger(y)
        && y >= 0 && y < modules.length && x >= 0 && x < modules.length && modules[y][x],
    };
  }

  async createDataUrl(value, width) {
    if (!Number.isInteger(width) || width <= 0) throw new RangeError("QR code width must be a positive integer");
    const code = this.createMatrix(value);
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Canvas 2D rendering is unavailable");
    canvas.width = width;
    canvas.height = width;
    const pixels = context.createImageData(width, width);
    const moduleCount = code.size + 2;
    for (let y = 0; y < width; y++) {
      const moduleY = Math.floor(y * moduleCount / width) - 1;
      for (let x = 0; x < width; x++) {
        const moduleX = Math.floor(x * moduleCount / width) - 1;
        const color = code.isDark(moduleX, moduleY) ? 0 : 255;
        pixels.data.set([color, color, color, 255], (y * width + x) * 4);
      }
    }
    context.putImageData(pixels, 0, 0);
    return canvas.toDataURL("image/png");
  }
}
