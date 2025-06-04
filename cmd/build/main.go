package main

import (
	"image"
	"log"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
)

func main() {
	rs := &winres.ResourceSet{}

	var images []image.Image
	for _, image := range []string{"icon_256.png", "icon_64.png", "icon_48.png", "icon_32.png", "icon_24.png", "icon_16.png"} {
		img, err := loadImage(filepath.Join("winres", image))
		if err != nil {
			log.Fatalf("failed to load image %q: %v", image, err)
		}
		images = append(images, img)
	}

	icon, err := winres.NewIconFromImages(images)
	if err != nil {
		log.Fatalf("failed to create icon from images: %v", err)
	}

	if err := rs.SetIcon(winres.RT_GROUP_ICON, icon); err != nil {
		log.Fatalf("failed to set icon: %v", err)
	}

	out, err := os.Create("cmd/mpvrc/rsrc_windows_amd64.syso")
	if err != nil {
		log.Fatalf("create file: %v", err)
	}
	if err = rs.WriteObject(out, winres.ArchAMD64); err != nil {
		log.Fatalf("write object: %v", err)
	}

	log.Print("build helper finished successfully")
}

func loadImage(name string) (image.Image, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}
