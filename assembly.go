package archorgdl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asticode/go-astiav"
)

func assembleSegments(ctx context.Context, segmentPaths []string, destination, workdir string, overwrite bool) error {
	if len(segmentPaths) == 0 {
		return errors.New("cannot assemble an empty segment list")
	}
	if _, err := os.Stat(destination); err == nil && !overwrite {
		return fmt.Errorf("%w: %s", ErrOutputExists, destination)
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return wrapError("create assembly directory", err)
	}
	manifest := filepath.Join(workdir, "filelist.ffconcat")
	if err := writeConcatManifest(manifest, workdir, segmentPaths); err != nil {
		return wrapError("write concat manifest", err)
	}
	partial := filepath.Join(workdir, "."+filepath.Base(destination)+".assembled.part")
	_ = os.Remove(partial)
	if err := remuxConcat(ctx, manifest, partial); err != nil {
		_ = os.Remove(partial)
		return err
	}
	if err := replaceFile(partial, destination, overwrite); err != nil {
		_ = os.Remove(partial)
		return err
	}
	return nil
}

func writeConcatManifest(filename, workdir string, segmentPaths []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteString("ffconcat version 1.0\n"); err != nil {
		return err
	}
	for _, segmentPath := range segmentPaths {
		relative, err := filepath.Rel(workdir, segmentPath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		escaped := strings.ReplaceAll(relative, "'", "'\\''")
		if _, err := fmt.Fprintf(file, "file '%s'\n", escaped); err != nil {
			return err
		}
	}
	return nil
}

func remuxConcat(ctx context.Context, manifest, destination string) error {
	input := astiav.AllocFormatContext()
	if input == nil {
		return errors.New("assembly: unable to allocate input format context")
	}
	defer input.Free()
	concatFormat := astiav.FindInputFormat("concat")
	if concatFormat == nil {
		return errors.New("assembly: FFmpeg concat demuxer is unavailable")
	}
	dictionary := astiav.NewDictionary()
	defer dictionary.Free()
	if err := dictionary.Set("safe", "0", astiav.NewDictionaryFlags()); err != nil {
		return fmt.Errorf("assembly: configure concat demuxer: %w", err)
	}
	if err := input.OpenInput(manifest, concatFormat, dictionary); err != nil {
		return fmt.Errorf("assembly: open concat manifest: %w", err)
	}
	defer input.CloseInput()
	if err := input.FindStreamInfo(nil); err != nil {
		return fmt.Errorf("assembly: find stream info: %w", err)
	}

	output, err := astiav.AllocOutputFormatContext(nil, "mp4", destination)
	if err != nil {
		return fmt.Errorf("assembly: allocate MP4 output: %w", err)
	}
	if output == nil {
		return errors.New("assembly: FFmpeg returned a nil output context")
	}
	defer output.Free()

	inputStreams := make(map[int]*astiav.Stream)
	outputStreams := make(map[int]*astiav.Stream)
	for _, inputStream := range input.Streams() {
		mediaType := inputStream.CodecParameters().MediaType()
		if mediaType != astiav.MediaTypeAudio && mediaType != astiav.MediaTypeVideo {
			continue
		}
		outputStream := output.NewStream(nil)
		if outputStream == nil {
			return errors.New("assembly: unable to allocate output stream")
		}
		if err := inputStream.CodecParameters().Copy(outputStream.CodecParameters()); err != nil {
			return fmt.Errorf("assembly: copy stream codec parameters: %w", err)
		}
		outputStream.CodecParameters().SetCodecTag(0)
		inputStreams[inputStream.Index()] = inputStream
		outputStreams[inputStream.Index()] = outputStream
	}
	if len(outputStreams) == 0 {
		return errors.New("assembly: concat input has no audio or video streams")
	}

	ioContext, err := astiav.OpenIOContext(destination, astiav.NewIOContextFlags(astiav.IOContextFlagWrite), nil, nil)
	if err != nil {
		return fmt.Errorf("assembly: open output: %w", err)
	}
	defer ioContext.Close()
	output.SetPb(ioContext)
	if err := output.WriteHeader(nil); err != nil {
		return fmt.Errorf("assembly: write MP4 header: %w", err)
	}

	packet := astiav.AllocPacket()
	if packet == nil {
		return errors.New("assembly: unable to allocate packet")
	}
	defer packet.Free()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := input.ReadFrame(packet); err != nil {
			if errors.Is(err, astiav.ErrEof) {
				break
			}
			return fmt.Errorf("assembly: read packet: %w", err)
		}
		inputStream, ok := inputStreams[packet.StreamIndex()]
		if !ok {
			packet.Unref()
			continue
		}
		outputStream := outputStreams[packet.StreamIndex()]
		packet.SetStreamIndex(outputStream.Index())
		packet.RescaleTs(inputStream.TimeBase(), outputStream.TimeBase())
		packet.SetPos(-1)
		if err := output.WriteInterleavedFrame(packet); err != nil {
			packet.Unref()
			return fmt.Errorf("assembly: write packet: %w", err)
		}
		packet.Unref()
	}
	if err := output.WriteTrailer(); err != nil {
		return fmt.Errorf("assembly: write MP4 trailer: %w", err)
	}
	if err := output.Flush(); err != nil {
		return fmt.Errorf("assembly: flush MP4 output: %w", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("assembly: output was not created: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("assembly: output is empty")
	}
	return nil
}
