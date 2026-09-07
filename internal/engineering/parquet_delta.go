package engineering

// guardDeltaLengths implements the DELTA_BINARY_PACKED length prefix of
// DELTA_LENGTH_BYTE_ARRAY. It walks bounded miniblocks without allocating from
// declared counts and checks every decoded length before the Parquet reader runs.
// Reference: Apache Parquet file-format/data-pages/encodings (delta encodings).
func guardDeltaLengths(data []byte, count, maxCell int) error {
	if count < 0 || count > 1000000 || maxCell < 1 || maxCell > 65536 {
		return ErrLimit
	}
	r := compactReader{data: data}
	block, err := r.unsigned()
	if err != nil || block < 128 || block > 65536 || block%128 != 0 {
		return ErrFormat
	}
	mini, err := r.unsigned()
	if err != nil || mini < 1 || mini > 256 || block%mini != 0 || (block/mini)%32 != 0 {
		return ErrFormat
	}
	total, err := r.unsigned()
	if err != nil || total != uint64(count) {
		return ErrFormat
	}
	first, err := r.unsigned()
	if err != nil {
		return ErrFormat
	}
	current := int64(first>>1) ^ -int64(first&1)
	var size int64
	produced := 0
	if count > 0 {
		if current < 0 || current > int64(maxCell) {
			return ErrLimit
		}
		size = current
		produced = 1
	}
	valuesPerMini := int(block) / int(mini)
	for produced < count {
		encoded, e := r.unsigned()
		if e != nil {
			return ErrFormat
		}
		minimum := int64(encoded>>1) ^ -int64(encoded&1)
		// Length differences are at most maxCell, while padded values never
		// contribute to the decoded length or allocate a synthetic string.
		if minimum < -int64(maxCell) || minimum > int64(maxCell) {
			return ErrFormat
		}
		widths, e := r.take(int(mini))
		if e != nil {
			return ErrFormat
		}
		for _, width := range widths {
			if produced >= count {
				break
			}
			if width > 32 {
				return ErrLimit
			}
			packed, e := r.take(valuesPerMini * int(width) / 8)
			if e != nil {
				return ErrFormat
			}
			for i := 0; i < valuesPerMini && produced < count; i++ {
				var delta int64
				for bit := 0; bit < int(width); bit++ {
					position := i*int(width) + bit
					delta |= int64((packed[position/8]>>uint(position%8))&1) << uint(bit)
				}
				current += minimum + delta
				if current < 0 || current > int64(maxCell) {
					return ErrLimit
				}
				size += current
				if size > int64(len(data)) {
					return ErrFormat
				}
				produced++
			}
		}
	}
	if size != int64(len(data)-r.at) {
		return ErrFormat
	}
	return nil
}
